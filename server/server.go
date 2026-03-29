package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ADS/AMS constants.
const AdsRouterAddr = "127.0.0.1:48898"

const (
	CmdReadDeviceInfo = 1
	CmdRead           = 2
	CmdWrite          = 3
	CmdReadState      = 4
	CmdWriteControl   = 5
	CmdReadWrite      = 9
)

const FlagResponse = 0x0001

// ADS error codes.
const ErrInvalidSize = 0x06

// Fixed frame sizes.
const (
	amsTCPHeaderSize = 6  // Reserved(2) + Length(4)
	amsHeaderSize    = 32 // full AmsHeader on the wire
	amsErrFieldSize  = 4  // ADS error code prepended to every response body
)

// pooledBuf is the unit of ownership passed through writeChan.
// Keeping the slice inside a struct lets the pool pointer stay stable even
// when append grows the backing array.
type pooledBuf struct {
	data []byte
}

// bufPool recycles response buffers. 512 bytes covers the fixed 42-byte header
// plus typical small payloads; oversized buffers are retained after the first use.
var bufPool = sync.Pool{
	New: func() any { return &pooledBuf{data: make([]byte, 0, 512)} },
}

// ReadFunc handles an ADS Read command.
// Fill buf with data and return (n, 0) on success, (0, adsErr) on failure.
type ReadFunc func(indexGroup, indexOffset uint32, buf []byte) (int, uint32)

// WriteFunc handles an ADS Write command.
// Return 0 on success, or an ADS error code on failure.
type WriteFunc func(indexGroup, indexOffset uint32, data []byte) uint32

// ReadWriteFunc handles an ADS ReadWrite command.
// Return (n, 0) on success, (0, adsErr) on failure.
type ReadWriteFunc func(indexGroup, indexOffset uint32, buf, data []byte) (int, uint32)

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

// Server implements an ADS device that registers with the local AMS router.
type Server struct {
	name  string
	port  uint16
	conn  net.Conn
	netid [6]byte

	onRead      ReadFunc
	onWrite     WriteFunc
	onReadWrite ReadWriteFunc

	log     *slog.Logger
	running atomic.Bool

	sem       chan struct{}   // limits handler concurrency
	writeChan chan *pooledBuf // serialises TCP writes; writer owns buf after receipt
	wg        sync.WaitGroup
}

// New creates a Server with the given device name and AMS port.
// Call SetRead / SetWrite / SetReadWrite before Start to register handlers.
func New(name string, port uint16) *Server {
	return &Server{
		name:      name,
		port:      port,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		sem:       make(chan struct{}, 8),
		writeChan: make(chan *pooledBuf, 256),
	}
}

// SetRead registers the handler for ADS Read commands.
func (server *Server) SetRead(fn ReadFunc) { server.onRead = fn }

// SetWrite registers the handler for ADS Write commands.
func (server *Server) SetWrite(fn WriteFunc) { server.onWrite = fn }

// SetReadWrite registers the handler for ADS ReadWrite commands.
func (server *Server) SetReadWrite(fn ReadWriteFunc) { server.onReadWrite = fn }

// SetLogLevel sets the minimum log level ("debug", "info", "warn", "error").
// An empty string silences all output. Must be called before Start.
func (server *Server) SetLogLevel(level string) {
	if level == "" {
		server.log = slog.New(slog.NewTextHandler(io.Discard, nil))
		return
	}
	if h, ok := server.log.Handler().(*logHandler); ok {
		h.mu.Lock()
		h.level = parseLevel(level)
		h.mu.Unlock()
		return
	}
	server.log = newLogger(level)
}

// SetLogFile tees log output to the given file path (appended).
// Pass an empty string to reset to stderr only.
func (server *Server) SetLogFile(path string) error {
	if path == "" {
		if h, ok := server.log.Handler().(*logHandler); ok {
			h.mu.Lock()
			h.out = os.Stderr
			h.mu.Unlock()
		}
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("SetLogFile: %w", err)
	}

	out := io.MultiWriter(os.Stderr, f)
	if h, ok := server.log.Handler().(*logHandler); ok {
		h.mu.Lock()
		h.out = out
		h.mu.Unlock()
		return nil
	}
	server.log = slog.New(&logHandler{level: slog.LevelInfo, out: out})
	return nil
}

// Start connects to the AMS router and begins serving ADS requests.
func (server *Server) Start() error {
	conn, err := net.Dial("tcp", AdsRouterAddr)
	if err != nil {
		return err
	}
	server.conn = conn

	if err := server.register(conn); err != nil {
		conn.Close()
		return err
	}

	server.log.Info("ADS server start", "server", server.name, "port", server.port)
	server.running.Store(true)

	go server.writer()
	go server.loop()
	return nil
}

// register sends the router registration handshake and stores the assigned NetID/port.
func (server *Server) register(conn net.Conn) error {
	var request [8]byte
	request[1] = 16
	request[2] = 2
	binary.LittleEndian.PutUint16(request[6:], server.port)

	if _, err := conn.Write(request[:]); err != nil {
		return err
	}

	var response [14]byte
	if _, err := io.ReadFull(conn, response[:]); err != nil {
		return err
	}

	copy(server.netid[:], response[6:12])
	server.port = binary.LittleEndian.Uint16(response[12:14])
	return nil
}

// Stop shuts the server down and waits for all in-flight handlers to finish.
func (server *Server) Stop() {
	server.running.Store(false)
	server.conn.Close()
	// Wait for all handlers before closing writeChan to prevent sends on a closed channel.
	server.wg.Wait()
	close(server.writeChan)
	server.log.Info("ADS server stop", "server", server.name)
}

// loop reads AMS packets from the TCP connection and dispatches each to a handler goroutine.
// No read deadline is set — the AMS router keeps the connection open indefinitely.
func (server *Server) loop() {
	// Stack-allocated header avoids a per-packet heap allocation for the 6-byte TCP prefix.
	var tcpHeader [amsTCPHeaderSize]byte

	for server.running.Load() {
		if _, err := io.ReadFull(server.conn, tcpHeader[:]); err != nil {
			if server.running.Load() {
				server.log.Error("TCP read error", "err", err)
			}
			return
		}
		pktLen := binary.LittleEndian.Uint32(tcpHeader[2:])

		payload := make([]byte, pktLen)
		if _, err := io.ReadFull(server.conn, payload); err != nil {
			if server.running.Load() {
				server.log.Error("payload read error", "err", err)
			}
			return
		}

		if len(payload) < amsHeaderSize {
			server.log.Warn("invalid AMS header: payload too short", "len", len(payload))
			continue
		}

		server.sem <- struct{}{} // backpressure: cap concurrent handlers
		server.wg.Add(1)
		go func(payload []byte) {
			defer func() {
				<-server.sem
				server.wg.Done()
			}()
			server.handlePayload(payload)
		}(payload)
	}
}

// writer drains writeChan, sends each buffer over TCP, then returns it to the pool.
// Owning all writes in one goroutine means s.conn needs no mutex.
func (server *Server) writer() {
	for pb := range server.writeChan {
		_, err := server.conn.Write(pb.data)
		pb.data = pb.data[:0]
		bufPool.Put(pb)
		if err != nil {
			server.log.Error("write error", "err", err)
			server.running.Store(false)
			server.conn.Close()
			return
		}
	}
}

// handlePayload parses the AMS header with direct slice reads (no reflection) and
// dispatches to the correct command handler.
func (server *Server) handlePayload(payload []byte) {
	var header AmsHeader
	copy(header.Target[:], payload[0:6])
	header.TargetPort = binary.LittleEndian.Uint16(payload[6:8])
	copy(header.Source[:], payload[8:14])
	header.SourcePort = binary.LittleEndian.Uint16(payload[14:16])
	header.Command = binary.LittleEndian.Uint16(payload[16:18])
	header.Flags = binary.LittleEndian.Uint16(payload[18:20])
	header.Length = binary.LittleEndian.Uint32(payload[20:24])
	header.Error = binary.LittleEndian.Uint32(payload[24:28])
	header.InvokeID = binary.LittleEndian.Uint32(payload[28:32])

	// Ignore response packets — the router reflects our own replies back and we
	// must not re-dispatch them as new incoming commands.
	if header.Flags&FlagResponse != 0 {
		return
	}

	switch header.Command {
	case CmdReadDeviceInfo:
		server.handleDeviceInfo(&header)

	case CmdReadState:
		server.handleReadState(&header)

	case CmdRead:
		server.handleRead(&header, payload)

	case CmdWrite:
		server.handleWrite(&header, payload)

	case CmdReadWrite:
		server.handleReadWrite(&header, payload)

	default:
		server.log.Error("unhandled command", "cmd", header.Command)
	}
}

func (server *Server) handleDeviceInfo(header *AmsHeader) {
	var body [24]byte
	body[0] = 1
	copy(body[4:], server.name)
	server.sendResponse(header, 0, body[:])
}

func (server *Server) handleReadState(header *AmsHeader) {
	var body [8]byte
	binary.LittleEndian.PutUint16(body[0:], 5) // ADS state: RUN
	binary.LittleEndian.PutUint16(body[2:], 1) // device state
	server.sendResponse(header, 0, body[:])
}

func (server *Server) handleRead(header *AmsHeader, payload []byte) {
	const minLen = amsHeaderSize + 12 // IndexGroup(4) + IndexOffset(4) + Length(4)

	if len(payload) < minLen {
		server.log.Warn("read: payload too short", "len", len(payload))
		server.sendResponse(header, ErrInvalidSize, nil)
		return
	}

	req := payload[amsHeaderSize:]
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	buf := make([]byte, length)
	if server.onRead != nil {
		n, adsErr := server.onRead(indexGroup, indexOffset, buf)
		if adsErr != 0 {
			server.log.Warn("read: handler error", "indexGroup", indexGroup, "indexOffset", indexOffset, "adsErr", adsErr)
			server.sendResponse(header, adsErr, nil)
			return
		}
		buf = buf[:n]
	}

	// Prepend the ReadLength field then the data in one allocation.
	body := binary.LittleEndian.AppendUint32(make([]byte, 0, 4+len(buf)), uint32(len(buf)))
	body = append(body, buf...)
	server.sendResponse(header, 0, body)
}

func (server *Server) handleWrite(header *AmsHeader, payload []byte) {
	const minLen = amsHeaderSize + 12 // IndexGroup(4) + IndexOffset(4) + Length(4)

	if len(payload) < minLen {
		server.log.Warn("write: payload too short", "len", len(payload))
		server.sendResponse(header, ErrInvalidSize, nil)
		return
	}

	req := payload[amsHeaderSize:]
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	writeLen := binary.LittleEndian.Uint32(req[8:12])

	if len(payload) < amsHeaderSize+12+int(writeLen) {
		server.log.Warn("write: declared length exceeds payload", "declared", writeLen, "actual", len(payload))
		server.sendResponse(header, ErrInvalidSize, nil)
		return
	}

	data := payload[amsHeaderSize+12 : amsHeaderSize+12+int(writeLen)]
	server.log.Info("write received", "indexGroup", indexGroup, "indexOffset", indexOffset, "length", writeLen)

	if server.onWrite != nil {
		if adsErr := server.onWrite(indexGroup, indexOffset, data); adsErr != 0 {
			server.log.Warn("write: handler error", "indexGroup", indexGroup, "indexOffset", indexOffset, "adsErr", adsErr)
			server.sendResponse(header, adsErr, nil)
			return
		}
	}

	server.sendResponse(header, 0, nil)
}

func (server *Server) handleReadWrite(header *AmsHeader, payload []byte) {
	const reqHdrLen = 16 // IndexGroup(4) + IndexOffset(4) + ReadLength(4) + WriteLength(4)

	if len(payload) < amsHeaderSize+reqHdrLen {
		server.log.Warn("readwrite: payload too short", "len", len(payload))
		server.sendResponse(header, ErrInvalidSize, nil)
		return
	}

	req := payload[amsHeaderSize:]
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	readLen := binary.LittleEndian.Uint32(req[8:12])
	writeLen := binary.LittleEndian.Uint32(req[12:16])

	if len(payload) < amsHeaderSize+reqHdrLen+int(writeLen) {
		server.log.Warn("readwrite: writeLen exceeds payload", "declared", writeLen, "actual", len(payload))
		server.sendResponse(header, ErrInvalidSize, nil)
		return
	}

	writeData := payload[amsHeaderSize+reqHdrLen : amsHeaderSize+reqHdrLen+int(writeLen)]
	readData := make([]byte, readLen)

	if server.onReadWrite != nil {
		n, adsErr := server.onReadWrite(indexGroup, indexOffset, readData, writeData)
		if adsErr != 0 {
			server.log.Warn("readwrite: handler error", "indexGroup", indexGroup, "indexOffset", indexOffset, "adsErr", adsErr)
			server.sendResponse(header, adsErr, nil)
			return
		}
		readData = readData[:n]
	}

	// Prepend the ReadLength field then the data in one allocation.
	body := binary.LittleEndian.AppendUint32(make([]byte, 0, 4+len(readData)), uint32(len(readData)))
	body = append(body, readData...)
	server.sendResponse(header, 0, body)
}

// sendResponse encodes a complete AMS response into a pooled buffer and enqueues
// it for the writer goroutine. Ownership of the buffer transfers to the writer,
// which returns it to the pool after the TCP send completes.
//
// Wire format:
//
//	AmsTcpHeader  (6 bytes)  — Reserved(2) + TotalAmsLength(4)
//	AmsHeader     (32 bytes) — Source/Target swapped, FlagResponse set
//	adsErr        (4 bytes)  — ADS result code
//	payload       (n bytes)  — command-specific response body
//
// If the write channel is full for 5 s the packet is dropped and the buffer
// is returned to the pool immediately.
func (server *Server) sendResponse(header *AmsHeader, adsErr uint32, payload []byte) {
	// Mutate header in place — caller does not use it after this point.
	header.Flags = FlagResponse | 0x0004
	header.Error = 0
	header.Length = amsErrFieldSize + uint32(len(payload))
	header.Target, header.Source = header.Source, header.Target
	header.TargetPort, header.SourcePort = header.SourcePort, header.TargetPort

	pb := bufPool.Get().(*pooledBuf)
	buffer := pb.data[:0]

	// AmsTcpHeader
	buffer = binary.LittleEndian.AppendUint16(buffer, 0) // Reserved
	buffer = binary.LittleEndian.AppendUint32(buffer, uint32(amsHeaderSize+amsErrFieldSize+len(payload)))

	// AmsHeader
	buffer = append(buffer, header.Target[:]...)
	buffer = binary.LittleEndian.AppendUint16(buffer, header.TargetPort)
	buffer = append(buffer, header.Source[:]...)
	buffer = binary.LittleEndian.AppendUint16(buffer, header.SourcePort)
	buffer = binary.LittleEndian.AppendUint16(buffer, header.Command)
	buffer = binary.LittleEndian.AppendUint16(buffer, header.Flags)
	buffer = binary.LittleEndian.AppendUint32(buffer, header.Length)
	buffer = binary.LittleEndian.AppendUint32(buffer, header.Error)
	buffer = binary.LittleEndian.AppendUint32(buffer, header.InvokeID)

	// Body
	buffer = binary.LittleEndian.AppendUint32(buffer, adsErr)
	buffer = append(buffer, payload...)

	pb.data = buffer

	// time.NewTimer reuses a single runtime timer object; time.After would allocate
	// a new goroutine and channel on every call.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case server.writeChan <- pb:
		// writer now owns pb and will return it to the pool.
	case <-timer.C:
		pb.data = pb.data[:0]
		bufPool.Put(pb)
		server.log.Error("sendResponse: write channel blocked, dropping packet",
			"invokeID", header.InvokeID,
			"cmd", header.Command,
			"queueLen", len(server.writeChan),
		)
	}
}

// logHandler is a minimal slog.Handler that writes human-readable lines to out.
type logHandler struct {
	level slog.Level
	mu    sync.Mutex
	out   io.Writer
}

func (h *logHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value.Any()))
		return true
	})
	msg := r.Message
	if len(attrs) > 0 {
		msg += " " + strings.Join(attrs, " ")
	}
	line := fmt.Sprintf("%s [server]  %-5s  %s\n",
		r.Time.Format("2006-01-02 15:04:05"),
		r.Level.String(),
		msg,
	)
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprint(h.out, line)
	return err
}

// WithAttrs and WithGroup are no-ops; the handler is intentionally flat.
func (h *logHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logHandler) WithGroup(string) slog.Handler      { return h }

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newLogger(level string) *slog.Logger {
	return slog.New(&logHandler{level: parseLevel(level), out: os.Stderr})
}
