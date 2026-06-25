package router

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

// ================= CLIENT =================
type Client struct {
	conn   net.Conn
	router *Router
	netId  AmsNetId
	port   uint16
	mu     sync.Mutex
}

func NewClient(conn net.Conn, r *Router) *Client {
	return &Client{
		conn:   conn,
		router: r,
	}
}

func (c *Client) Run() {
	defer c.shutdown()

	if err := c.handleRegister(); err != nil {
		log.Println("Register failed:", err)
		return
	}

	c.readLoop()
}

// used for outgoing connections
func (c *Client) RunWithoutRegister() {
	defer c.shutdown()
	c.readLoop()
}

func (c *Client) readLoop() {
	for {
		header := make([]byte, 38)

		_, err := io.ReadFull(c.conn, header)
		if err != nil {
			return
		}

		length := binary.LittleEndian.Uint32(header[2:6])

		payload := make([]byte, length)
		_, err = io.ReadFull(c.conn, payload)
		if err != nil {
			return
		}

		c.handlePacket(header, payload)
	}
}

// REGISTER HANDSHAKE
func (c *Client) handleRegister() error {
	var req [8]byte

	_, err := io.ReadFull(c.conn, req[:])
	if err != nil {
		return err
	}

	if req[1] != 0x10 || req[2] != 0x02 {
		return fmt.Errorf("invalid register command")
	}

	c.port = binary.LittleEndian.Uint16(req[6:8])

	c.netId = c.router.localNetId

	var res [14]byte
	res[1] = 0x10
	res[2] = 0x02

	copy(res[6:12], c.netId[:])
	binary.LittleEndian.PutUint16(res[12:14], c.port)

	_, err = c.conn.Write(res[:])
	if err != nil {
		return err
	}

	c.router.Register(c.netId, c)

	log.Printf("Registered client: %v port=%d\n", c.netId, c.port)

	return nil
}

func (c *Client) handlePacket(header, payload []byte) {
	dest := parseNetId(header[12:18])
	full := append(header, payload...)

	if err := c.router.Forward(dest, full); err != nil {
		log.Println("Forward error:", err)
	}
}

func (c *Client) Send(data []byte) error {
	_, err := c.conn.Write(data)
	return err
}

func (c *Client) shutdown() {
	if c.netId != (AmsNetId{}) {
		c.router.Unregister(c.netId, c)
	}
	c.router.RemoveClient(c)
	c.conn.Close()
}
