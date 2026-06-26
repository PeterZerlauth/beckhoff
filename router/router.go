package router

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/PeterZerlauth/beckhoff/ams"
)


// ================= ROUTER =================
type Router struct {
	mu sync.RWMutex

	localNetId ams.NetId

	routes  map[ams.NetId]*Client
	clients map[*Client]struct{}

	listener net.Listener
}

func NewRouter() *Router {
	return &Router{
		routes:  make(map[ams.NetId]*Client),
		clients: make(map[*Client]struct{}),
	}
}

func (r *Router) SetRoutes(cfg *Config) error {
	netid, err := parseNetIdString(cfg.AmsRouter.NetId)
	if err != nil {
		return err
	}

	r.localNetId = netid
	log.Println("Local NetID:", netid)

	return nil
}

func (r *Router) Start() error {
	// Load config
	cfg, err := LoadConfig("settings.json")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set local NetID only
	netid, err := parseNetIdString(cfg.AmsRouter.NetId)
	if err != nil {
		return fmt.Errorf("failed to init router: %w", err)
	}

	r.localNetId = netid
	log.Println("Local NetID:", netid)

	// ADS Router port
	address := "127.0.0.1:48898"

	// Start TCP listener
	ln, err := net.Listen("tcp", address)
	if err != nil {
		if errors.Is(err, syscall.Errno(10013)) {
			log.Println("ADS Router disabled: port 48898 cannot be bound")
			return nil
		}
		return fmt.Errorf("failed to start ADS router: %w", err)
	}

	r.listener = ln

	log.Println("ADS Router started")

	// Connect remotes only after listener is running
	for _, rc := range cfg.AmsRouter.RemoteConnections {
		go r.connectRemote(rc)
	}

	go r.acceptLoop()

	return nil
}

func (r *Router) acceptLoop() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}

		client := NewClient(conn, r)

		r.clients[client] = struct{}{}

		log.Println("New connection:", conn.RemoteAddr())

		go client.Run()
	}
}

func (r *Router) Register(id ams.NetId, c *Client) {
	r.routes[id] = c
	log.Printf("Ads route: %v %v -> %s\n", r.localNetId, id, c.conn.RemoteAddr())
}

func (r *Router) Unregister(id ams.NetId, c *Client) {
	if existing, ok := r.routes[id]; ok && existing == c {
		delete(r.routes, id)
	}
}

func (r *Router) Forward(dest ams.NetId, data []byte) error {
	client, ok := r.routes[dest]

	if !ok {
		return fmt.Errorf("destination not found: %v", dest)
	}

	done := make(chan error, 1)
	go func() {
		done <- client.Send(data)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending to %v", dest)
	}
}

func (r *Router) RemoveClient(c *Client) {

	delete(r.clients, c)

	for id, client := range r.routes {
		if client == c {
			delete(r.routes, id)
		}
	}
}

// ================= REMOTE CONNECT =================

func (r *Router) connectRemote(rc RemoteConnection) {
	if rc.Type != "TCP_IP" {
		return
	}

	addr := net.JoinHostPort(rc.Address, "48898")

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Println("Invalid route:", rc.Name, rc.NetId, rc.Address)
		return
	}

	client := NewClient(conn, r)

	netid, err := parseNetIdString(rc.NetId)
	if err != nil {
		log.Println("Invalid NetID:", rc.NetId)
		return
	}

	client.netId = netid

	r.clients[client] = struct{}{}
	r.routes[netid] = client
	log.Println("Connected remote:", rc.Name, addr)

	go client.RunWithoutRegister() // important
}

// ================= HELPERS =================

func parseNetId(b []byte) ams.NetId {
	var id ams.NetId
	copy(id[:], b)
	return id
}

func parseNetIdString(s string) (ams.NetId, error) {
	var id ams.NetId
	var b [6]byte

	n, err := fmt.Sscanf(s, "%d.%d.%d.%d.%d.%d",
		&b[0], &b[1], &b[2], &b[3], &b[4], &b[5])

	if err != nil || n != 6 {
		return id, fmt.Errorf("invalid NetID: %s", s)
	}

	copy(id[:], b[:])
	return id, nil
}
