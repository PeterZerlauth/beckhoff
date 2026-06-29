package router

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/PeterZerlauth/beckhoff/ams"
	"github.com/PeterZerlauth/beckhoff/logger"
)

// Router
type Router struct {
	mu sync.RWMutex

	localNetId ams.NetId

	routes  map[ams.NetId]*Client
	clients map[*Client]struct{}

	listener net.Listener
	log      *slog.Logger
}

// Create new ads router
func NewRouter() *Router {
	return &Router{
		routes:  make(map[ams.NetId]*Client),
		clients: make(map[*Client]struct{}),
		log:     logger.GetLogger("", 7).Log(), // ✅ singleton logger
	}
}

// Set routes
func (r *Router) SetRoutes(cfg *Config) error {
	netid, err := ams.ParseNetId(cfg.AmsRouter.NetId)
	if err != nil {
		return err
	}

	r.localNetId = netid
	r.log.Info("Local NetID", "netid", netid)

	return nil
}

// Start router
func (r *Router) Start() error {
	cfg, err := LoadConfig("settings.json")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	netid, err := ams.ParseNetId(cfg.AmsRouter.NetId)
	if err != nil {
		return fmt.Errorf("failed to init router: %w", err)
	}

	r.localNetId = netid
	r.log.Info("Local NetID", "netid", netid)

	address := "127.0.0.1:48898"

	ln, err := net.Listen("tcp", address)
	if err != nil {
		if errors.Is(err, syscall.Errno(10013)) {
			r.log.Warn("ADS Router disabled: port bind failed")
			return nil
		}
		return fmt.Errorf("failed to start ADS router: %w", err)
	}

	r.listener = ln
	r.log.Info("ADS Router started", "address", address)

	for _, rc := range cfg.AmsRouter.RemoteConnections {
		go r.connectRemote(rc)
	}

	go r.acceptLoop()

	return nil
}

// Accept clients
func (r *Router) acceptLoop() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			r.log.Error("Accept error", "error", err)
			continue
		}

		client := NewClient(conn, r)

		r.mu.Lock()
		r.clients[client] = struct{}{}
		r.mu.Unlock()

		r.log.Info("New connection", "addr", conn.RemoteAddr())

		go client.Run()
	}
}

// Register a client
func (r *Router) Register(id ams.NetId, c *Client) {
	r.mu.Lock()
	r.routes[id] = c
	r.mu.Unlock()

	r.log.Info("Route registered",
		"local", r.localNetId,
		"remote", id,
		"addr", c.conn.RemoteAddr(),
	)
}

// Unregister a client
func (r *Router) Unregister(id ams.NetId, c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.routes[id]; ok && existing == c {
		delete(r.routes, id)
	}
}

// Forward data
func (r *Router) Forward(dest ams.NetId, data []byte) error {
	r.mu.RLock()
	client, ok := r.routes[dest]
	r.mu.RUnlock()

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
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, c)

	for id, client := range r.routes {
		if client == c {
			delete(r.routes, id)
		}
	}
}

// Remote connection
func (r *Router) connectRemote(rc RemoteConnection) {
	if rc.Type != "TCP_IP" {
		return
	}

	addr := net.JoinHostPort(rc.Address, "48898")

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		r.log.Warn("Invalid route",
			"name", rc.Name,
			"netid", rc.NetId,
			"address", rc.Address,
		)
		return
	}

	client := NewClient(conn, r)

	netid, err := ams.ParseNetId(rc.NetId)
	if err != nil {
		r.log.Warn("Invalid NetID", "netid", rc.NetId)
		return
	}

	client.netId = netid

	r.mu.Lock()
	r.clients[client] = struct{}{}
	r.routes[netid] = client
	r.mu.Unlock()

	r.log.Info("Connected remote",
		"name", rc.Name,
		"addr", addr,
	)

	go client.RunWithoutRegister()
}
