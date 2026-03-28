package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"runtime"
	"sync"
)

////////////////////////////////////////////////////////////////////////////////
// ADS/AMS constants
////////////////////////////////////////////////////////////////////////////////

const AdsRouterAddr = "127.0.0.1:48898" // Default TwinCAT ADS router address

// ADS/AMS commands
const (
	CmdReadDeviceInfo = 1
	CmdRead           = 2
	CmdWrite          = 3
	CmdReadState      = 4
	CmdWriteControl   = 5
	CmdReadWrite      = 9
)

const FlagResponse = 0x0001 // Response flag

////////////////////////////////////////////////////////////////////////////////
// Data structures
////////////////////////////////////////////////////////////////////////////////

// AmsTcpHeader represents the TCP header for ADS/AMS messages
type AmsTcpHeader struct {
	Reserved uint16 // always 0
	Length   uint32 // length of AMS payload
}

// AmsHeader represents the AMS protocol header
type AmsHeader struct {
	Target     [6]byte
	TargetPort uint16
	Source     [6]byte
	SourcePort uint16
	Command    uint16
	Flags      uint16
	Length     uint32
	Error      uint32
	InvokeID   uint32
}

// Packet wraps a TCP+AMS payload for the worker queue
type Packet struct {
	tcp     AmsTcpHeader
	payload []byte
}

////////////////////////////////////////////////////////////////////////////////
// Server definition
////////////////////////////////////////////////////////////////////////////////

// Server represents an ADS server instance
type Server struct {
	name    string       // friendly server name
	port    uint16       // ADS port number
	conn    net.Conn     // TCP connection to ADS router
	netid   [6]byte      // assigned by router
	mu      sync.Mutex   // ensures thread-safe writes
	running bool         // controls main loop
	jobs    chan Packet  // queue for worker pool
	workers int          // number of worker goroutines
}

////////////////////////////////////////////////////////////////////////////////
// Constructor
////////////////////////////////////////////////////////////////////////////////

// New creates a new ADS server with a name and port
func New(name string, port uint16) *Server {
	return &Server{
		name:    name,
		port:    port,
		jobs:    make(chan Packet, 256), // buffered channel
		workers: runtime.NumCPU(),        // default worker count = CPU cores
	}
}

////////////////////////////////////////////////////////////////////////////////
// Start server
////////////////////////////////////////////////////////////////////////////////

func (s *Server) Start() error {
	// Connect to ADS router (TwinCAT)
	conn, err := net.Dial("tcp", AdsRouterAddr)
	if err != nil {
		return err
	}
	s.conn = conn

	// Register server port with ADS router
	req := make([]byte, 8)
	req[0] = 0
	req[1] = 16
	req[2] = 2
	binary.LittleEndian.PutUint16(req[6:], s.port)

	if _, err := conn.Write(req); err != nil {
		return err
	}

	// Read router response: netid + assigned port
	resp := make([]byte, 14)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	copy(s.netid[:], resp[6:12])
	s.port = binary.LittleEndian.Uint16(resp[12:14])

	log.Printf("%s registered with ADS router on port %d", s.name, s.port)

	// Start worker pool
	for i := 0; i < s.workers; i++ {
		go s.worker(i)
	}

	// Start main packet reading loop
	s.running = true
	go s.loop()

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Stop server
////////////////////////////////////////////////////////////////////////////////

func (s *Server) Stop() {
	s.running = false
	close(s.jobs) // stop workers
	if s.conn != nil {
		s.conn.Close()
	}
	log.Printf("%s stopped", s.name)
}

////////////////////////////////////////////////////////////////////////////////
// Main loop: reads packets from TCP connection
////////////////////////////////////////////////////////////////////////////////

func (s *Server) loop() {
	for s.running {
		// Read TCP header
		var tcp AmsTcpHeader
		if err := binary.Read(s.conn, binary.LittleEndian, &tcp); err != nil {
			log.Println("TCP read error:", err)
			return
		}

		// Read AMS payload
		payload := make([]byte, tcp.Length)
		if _, err := io.ReadFull(s.conn, payload); err != nil {
			log.Println("Payload read error:", err)
			return
		}

		if len(payload) < 32 {
			log.Println("Invalid AMS header")
			return
		}

		// Enqueue packet for parallel processing
		s.jobs <- Packet{tcp: tcp, payload: payload}
	}
}

////////////////////////////////////////////////////////////////////////////////
// Worker pool: handles packets concurrently
////////////////////////////////////////////////////////////////////////////////

func (s *Server) worker(id int) {
	log.Printf("Worker %d started", id)
	for pkt := range s.jobs {
		s.handlePacket(pkt)
	}
}

////////////////////////////////////////////////////////////////////////////////
// Packet dispatcher
////////////////////////////////////////////////////////////////////////////////

func (s *Server) handlePacket(pkt Packet) {
	payload := pkt.payload

	var hdr AmsHeader
	if err := binary.Read(bytes.NewReader(payload[:32]), binary.LittleEndian, &hdr); err != nil {
		log.Println("Header parse error:", err)
		return
	}

	switch hdr.Command {
	case CmdReadDeviceInfo:
		s.handleDeviceInfo(&hdr)
	case CmdReadState:
		s.handleReadState(&hdr)
	case CmdRead:
		s.handleRead(&hdr, payload)
	case CmdWrite:
		s.handleWrite(&hdr, payload)
	case CmdReadWrite:
		s.handleReadWrite(&hdr, payload)
	default:
		log.Println("Unhandled command:", hdr.Command)
	}
}

////////////////////////////////////////////////////////////////////////////////
// ADS Handlers
////////////////////////////////////////////////////////////////////////////////

// handleDeviceInfo returns server info (version + name)
func (s *Server) handleDeviceInfo(hdr *AmsHeader) {
	data := make([]byte, 24)
	data[0] = 1 // major version
	data[1] = 0 // minor version
	copy(data[4:], []byte(s.name)) // include friendly server name

	resp := append(u32(0), data...) // Error code + device info
	s.sendResponse(hdr, resp)
}

// handleReadState returns server run state
func (s *Server) handleReadState(hdr *AmsHeader) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint16(data[0:], 5) // ADS run state
	binary.LittleEndian.PutUint16(data[2:], 1) // device state

	resp := append(u32(0), data...)
	s.sendResponse(hdr, resp)
}

// handleRead fills buffer with example pattern
func (s *Server) handleRead(hdr *AmsHeader, payload []byte) {
	req := payload[32:]
	length := binary.LittleEndian.Uint32(req[8:12])

	data := make([]byte, length)
	for i := range data {
		data[i] = 0xAA
	}

	resp := append(u32(0), u32(length)...)
	resp = append(resp, data...)
	s.sendResponse(hdr, resp)
}

// handleWrite acknowledges write (stub)
func (s *Server) handleWrite(hdr *AmsHeader, payload []byte) {
	log.Printf("Write received: length=%d", len(payload))
	s.sendResponse(hdr, u32(0))
}

// handleReadWrite echoes write data into read buffer
func (s *Server) handleReadWrite(hdr *AmsHeader, payload []byte) {
	req := payload[32:]
	readLen := binary.LittleEndian.Uint32(req[8:12])
	writeLen := binary.LittleEndian.Uint32(req[12:16])

	writeData := payload[32+16 : 32+16+writeLen]
	readData := make([]byte, readLen)

	n := min(int(readLen), int(writeLen))
	copy(readData[:n], writeData[:n])

	resp := append(u32(0), u32(readLen)...)
	resp = append(resp, readData...)
	s.sendResponse(hdr, resp)
}

////////////////////////////////////////////////////////////////////////////////
// Send response (thread-safe)
////////////////////////////////////////////////////////////////////////////////

func (s *Server) sendResponse(hdr *AmsHeader, data []byte) {
	hdr.Flags = FlagResponse
	hdr.Length = uint32(len(data))
	hdr.Error = 0

	// Swap source/target
	hdr.Target, hdr.Source = hdr.Source, hdr.Target
	hdr.TargetPort, hdr.SourcePort = hdr.SourcePort, hdr.TargetPort

	var buf bytes.Buffer
	tcp := AmsTcpHeader{
		Reserved: 0,
		Length:   uint32(32 + len(data)),
	}

	binary.Write(&buf, binary.LittleEndian, tcp)
	binary.Write(&buf, binary.LittleEndian, hdr)
	buf.Write(data)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conn.Write(buf.Bytes()); err != nil {
		log.Println("Write error:", err)
	}
}

////////////////////////////////////////////////////////////////////////////////
// Helper functions
////////////////////////////////////////////////////////////////////////////////

func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

////////////////////////////////////////////////////////////////////////////////
// Entry point
////////////////////////////////////////////////////////////////////////////////

func main() {
	s := New("My Server", 25000)

	if err := s.Start(); err != nil {
		log.Fatal(err)
	}

	log.Printf("%s running (Ctrl+C to stop)", s.name)
	select {} // block forever
}