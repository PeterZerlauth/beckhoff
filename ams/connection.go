package ams

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync"

)

type Handler interface {
	HandlePacket(req []byte) ([]byte, error)
}

type Connection struct {
	conn net.Conn

	port  uint16
	netid NetId

	handler Handler
	log     *slog.Logger

	jobs chan []byte

	wmu sync.Mutex
}

var bufPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 1024)
	},
}

func NewConnection(port uint16, h Handler, log *slog.Logger) *Connection {
	return &Connection{
		port:    port,
		handler: h,
		log:     log,
		jobs:    make(chan []byte, 1024),
	}
}

func (c *Connection) Start() error {
	conn, err := net.Dial("tcp", "127.0.0.1:48898")
	if err != nil {
		c.log.Error("connection failed", "error", err)
		return err
	}
	c.conn = conn

	if err := c.register(); err != nil {
		c.log.Error("register failed", "error", err)
		return err
	}

	for i := 0; i < 4; i++ {
		go c.worker()
	}

	go c.loop()
	return nil
}

func (c *Connection) register() error {
	var req [8]byte
	req[1] = 16
	req[2] = 2
	binary.LittleEndian.PutUint16(req[6:], c.port)

	if _, err := c.conn.Write(req[:]); err != nil {
		c.log.Error("Write failed", "error", err)
		return err
	}

	var res [14]byte
	if _, err := io.ReadFull(c.conn, res[:]); err != nil {
		c.log.Error("Read failed", "error", err)
		return err
	}

	copy(c.netid[:], res[6:12])
	c.port = binary.LittleEndian.Uint16(res[12:14])

	return nil
}

func (c *Connection) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Connection) loop() {
	for {
		p, err := c.readPacket()
		if err != nil {
			c.log.Error("connection closed", "err", err)
			return
		}
		c.jobs <- p
	}
}

func (c *Connection) worker() {
	for p := range c.jobs {
		resp, err := c.handler.HandlePacket(p)
		if err == nil && resp != nil {
			c.send(resp)
		}
		bufPool.Put(p[:0])
	}
}

func (c *Connection) readPacket() ([]byte, error) {
	var tcp [TcpHeaderSize]byte

	if _, err := io.ReadFull(c.conn, tcp[:]); err != nil {
		c.log.Error("Read failed", "error", err)
		return nil, err
	}

	length := binary.LittleEndian.Uint32(tcp[2:])
	buf := bufPool.Get().([]byte)

	if cap(buf) < int(length) {
		buf = make([]byte, length)
	}
	buf = buf[:length]

	if _, err := io.ReadFull(c.conn, buf); err != nil {
		c.log.Error("Read failed", "error", err)
		return nil, err
	}

	return buf, nil
}

func (c *Connection) send(buf []byte) {
	c.wmu.Lock()
	_, _ = c.conn.Write(buf)
	c.wmu.Unlock()
}

func (c *Connection) NetID() string {
	return c.netid.String()
}

func (c *Connection) Port() uint16 {
	return c.port
}
