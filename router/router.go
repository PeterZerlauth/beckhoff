package router

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"syscall"
	"time"
)

type AmsNetId [6]byte

// ================= ROUTER =================
type Router struct {
	mu sync.RWMutex

	localNetId AmsNetId

	routes  map[AmsNetId]*Client
	clients map[*Client]struct{}

	listener net.Listener
}

func NewRouter() *Router {
	return &Router{
		routes:  make(map[AmsNetId]*Client),
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

	for _, rc := range cfg.AmsRouter.RemoteConnections {
		go r.connectRemote(rc)
	}

	return nil
}
func (r *Router) Start() error {

	// ✅ 1. Load config first
	cfg, err := LoadConfig("settings.json")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// ✅ 2. Initialize router (NetID + remote connections)
	if err := r.SetRoutes(cfg); err != nil {
		return fmt.Errorf("failed to init router: %w", err)
	}

	address := "127.0.0.1:48898"

	// ✅ 3. Start TCP listener
	ln, err := net.Listen("tcp", address)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			log.Println("ADS Router already running on", address)
			return nil
		}
		return err
	}

	r.listener = ln

	log.Println("ADS Router listening on", address)

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

func (r *Router) Register(id AmsNetId, c *Client) {
	r.routes[id] = c
	log.Printf("Route registered: %v %v -> %s\n", r.localNetId, id, c.conn.RemoteAddr())
}

func (r *Router) Unregister(id AmsNetId, c *Client) {
	if existing, ok := r.routes[id]; ok && existing == c {
		delete(r.routes, id)
	}
}

func (r *Router) Forward(dest AmsNetId, data []byte) error {
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

func parseNetId(b []byte) AmsNetId {
	var id AmsNetId
	copy(id[:], b)
	return id
}

func parseNetIdString(s string) (AmsNetId, error) {
	var id AmsNetId
	var b [6]byte

	n, err := fmt.Sscanf(s, "%d.%d.%d.%d.%d.%d",
		&b[0], &b[1], &b[2], &b[3], &b[4], &b[5])

	if err != nil || n != 6 {
		return id, fmt.Errorf("invalid NetID: %s", s)
	}

	copy(id[:], b[:])
	return id, nil
}

func generateNetId(conn net.Conn) AmsNetId {
	var id AmsNetId

	ip := conn.RemoteAddr().(*net.TCPAddr).IP.To4()
	if ip != nil {
		copy(id[0:4], ip)
	}

	now := time.Now().UnixNano()
	id[4] = byte(now)
	id[5] = byte(now >> 8)

	return id
}
