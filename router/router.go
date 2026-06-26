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


// Router
type Router struct {
	mu sync.RWMutex

	localNetId ams.NetId

	routes  map[ams.NetId]*Client
	clients map[*Client]struct{}

	listener net.Listener
}

// Create new ads router
func NewRouter() *Router {
	return &Router{
		routes:  make(map[ams.NetId]*Client),
		clients: make(map[*Client]struct{}),
	}
}

// Set routeres
func (r *Router) SetRoutes(cfg *Config) error {
	netid, err := ams.ParseNetId(cfg.AmsRouter.NetId)
	if err != nil {
		return err
	}

	r.localNetId = netid
	log.Println("Local NetID:", netid)

	return nil
}

// Start router 
func (r *Router) Start() error {
	// Load config
	cfg, err := LoadConfig("settings.json")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Set local NetID only
	netid, err := ams.ParseNetId(cfg.AmsRouter.NetId)
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

// Acccept clients
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

// Register a client
func (r *Router) Register(id ams.NetId, c *Client) {
	r.routes[id] = c
	log.Printf("Ads route: %v %v -> %s\n", r.localNetId, id, c.conn.RemoteAddr())
}

// Unregister a client
func (r *Router) Unregister(id ams.NetId, c *Client) {
	if existing, ok := r.routes[id]; ok && existing == c {
		delete(r.routes, id)
	}
}

// Forward data
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

// Remove a client
func (r *Router) RemoveClient(c *Client) {

	delete(r.clients, c)

	for id, client := range r.routes {
		if client == c {
			delete(r.routes, id)
		}
	}
}

// remote connection
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

	netid, err := ams.ParseNetId(rc.NetId)
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
