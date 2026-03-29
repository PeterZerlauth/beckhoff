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
const adsErrInvalidSize = 0x06

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
func (s *Server) SetRead(fn ReadFunc) { s.onRead = fn }

// SetWrite registers the handler for ADS Write commands.
func (s *Server) SetWrite(fn WriteFunc) { s.onWrite = fn }

// SetReadWrite registers the handler for ADS ReadWrite commands.
func (s *Server) SetReadWrite(fn ReadWriteFunc) { s.onReadWrite = fn }

// SetLogLevel sets the minimum log level ("debug", "info", "warn", "error").
// An empty string silences all output. Must be called before Start.
func (s *Server) SetLogLevel(level string) {
	if level == "" {
		s.log = slog.New(slog.NewTextHandler(io.Discard, nil))
		return
	}
	if h, ok := s.log.Handler().(*logHandler); ok {
		h.mu.Lock()
		h.level = parseLevel(level)
		h.mu.Unlock()
		return
	}
	s.log = newLogger(level)
}

// SetLogFile tees log output to the given file path (appended).
// Pass an empty string to reset to stderr only.
func (s *Server) SetLogFile(path string) error {
	if path == "" {
		if h, ok := s.log.Handler().(*logHandler); ok {
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
	if h, ok := s.log.Handler().(*logHandler); ok {
		h.mu.Lock()
		h.out = out
		h.mu.Unlock()
		return nil
	}
	s.log = slog.New(&logHandler{level: slog.LevelInfo, out: out})
	return nil
}

// Start connects to the AMS router and begins serving ADS requests.
func (s *Server) Start() error {
	conn, err := net.Dial("tcp", AdsRouterAddr)
	if err != nil {
		return err
	}
	s.conn = conn

	if err := s.register(conn); err != nil {
		conn.Close()
		return err
	}

	s.log.Info("registered on ADS router", "server", s.name, "port", s.port)
	s.running.Store(true)

	go s.writer()
	go s.loop()
	return nil
}

// register sends the router registration handshake and stores the assigned NetID/port.
func (s *Server) register(conn net.Conn) error {
	var req [8]byte
	req[1] = 16
	req[2] = 2
	binary.LittleEndian.PutUint16(req[6:], s.port)

	if _, err := conn.Write(req[:]); err != nil {
		return err
	}

	var resp [14]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return err
	}

	copy(s.netid[:], resp[6:12])
	s.port = binary.LittleEndian.Uint16(resp[12:14])
	return nil
}

// Stop shuts the server down and waits for all in-flight handlers to finish.
func (s *Server) Stop() {
	s.running.Store(false)
	s.conn.Close()
	// Wait for all handlers before closing writeChan to prevent sends on a closed channel.
	s.wg.Wait()
	close(s.writeChan)
	s.log.Info("stopped", "server", s.name)
}

// loop reads AMS packets from the TCP connection and dispatches each to a handler goroutine.
// No read deadline is set — the AMS router keeps the connection open indefinitely.
func (s *Server) loop() {
	// Stack-allocated header avoids a per-packet heap allocation for the 6-byte TCP prefix.
	var tcpHdr [amsTCPHeaderSize]byte

	for s.running.Load() {
		if _, err := io.ReadFull(s.conn, tcpHdr[:]); err != nil {
			if s.running.Load() {
				s.log.Error("TCP read error", "err", err)
			}
			return
		}
		pktLen := binary.LittleEndian.Uint32(tcpHdr[2:])

		payload := make([]byte, pktLen)
		if _, err := io.ReadFull(s.conn, payload); err != nil {
			if s.running.Load() {
				s.log.Error("payload read error", "err", err)
			}
			return
		}

		if len(payload) < amsHeaderSize {
			s.log.Warn("invalid AMS header: payload too short", "len", len(payload))
			continue
		}

		s.sem <- struct{}{} // backpressure: cap concurrent handlers
		s.wg.Add(1)
		go func(payload []byte) {
			defer func() {
				<-s.sem
				s.wg.Done()
			}()
			s.handlePayload(payload)
		}(payload)
	}
}

// writer drains writeChan, sends each buffer over TCP, then returns it to the pool.
// Owning all writes in one goroutine means s.conn needs no mutex.
func (s *Server) writer() {
	for pb := range s.writeChan {
		_, err := s.conn.Write(pb.data)
		pb.data = pb.data[:0]
		bufPool.Put(pb)
		if err != nil {
			s.log.Error("write error", "err", err)
			s.running.Store(false)
			s.conn.Close()
			return
		}
	}
}

// handlePayload parses the AMS header with direct slice reads (no reflection) and
// dispatches to the correct command handler.
func (s *Server) handlePayload(payload []byte) {
	var hdr AmsHeader
	copy(hdr.Target[:], payload[0:6])
	hdr.TargetPort = binary.LittleEndian.Uint16(payload[6:8])
	copy(hdr.Source[:], payload[8:14])
	hdr.SourcePort = binary.LittleEndian.Uint16(payload[14:16])
	hdr.Command = binary.LittleEndian.Uint16(payload[16:18])
	hdr.Flags = binary.LittleEndian.Uint16(payload[18:20])
	hdr.Length = binary.LittleEndian.Uint32(payload[20:24])
	hdr.Error = binary.LittleEndian.Uint32(payload[24:28])
	hdr.InvokeID = binary.LittleEndian.Uint32(payload[28:32])

	// Ignore response packets — the router reflects our own replies back and we
	// must not re-dispatch them as new incoming commands.
	if hdr.Flags&FlagResponse != 0 {
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
		s.log.Error("unhandled command", "cmd", hdr.Command)
	}
}

func (s *Server) handleDeviceInfo(hdr *AmsHeader) {
	var body [24]byte
	body[0] = 1
	copy(body[4:], s.name)
	s.sendResponse(hdr, 0, body[:])
}

func (s *Server) handleReadState(hdr *AmsHeader) {
	var body [8]byte
	binary.LittleEndian.PutUint16(body[0:], 5) // ADS state: RUN
	binary.LittleEndian.PutUint16(body[2:], 1) // device state
	s.sendResponse(hdr, 0, body[:])
}

func (s *Server) handleRead(hdr *AmsHeader, payload []byte) {
	const minLen = amsHeaderSize + 12 // IndexGroup(4) + IndexOffset(4) + Length(4)

	if len(payload) < minLen {
		s.log.Warn("read: payload too short", "len", len(payload))
		s.sendResponse(hdr, adsErrInvalidSize, nil)
		return
	}

	req := payload[amsHeaderSize:]
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	buf := make([]byte, length)
	if s.onRead != nil {
		n, adsErr := s.onRead(indexGroup, indexOffset, buf)
		if adsErr != 0 {
			s.log.Warn("read: handler error", "indexGroup", indexGroup, "indexOffset", indexOffset, "adsErr", adsErr)
			s.sendResponse(hdr, adsErr, nil)
			return
		}
		buf = buf[:n]
	}

	// Prepend the ReadLength field then the data in one allocation.
	body := binary.LittleEndian.AppendUint32(make([]byte, 0, 4+len(buf)), uint32(len(buf)))
	body = append(body, buf...)
	s.sendResponse(hdr, 0, body)
}

func (s *Server) handleWrite(hdr *AmsHeader, payload []byte) {
	const minLen = amsHeaderSize + 12 // IndexGroup(4) + IndexOffset(4) + Length(4)

	if len(payload) < minLen {
		s.log.Warn("write: payload too short", "len", len(payload))
		s.sendResponse(hdr, adsErrInvalidSize, nil)
		return
	}

	req := payload[amsHeaderSize:]
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	writeLen := binary.LittleEndian.Uint32(req[8:12])

	if len(payload) < amsHeaderSize+12+int(writeLen) {
		s.log.Warn("write: declared length exceeds payload", "declared", writeLen, "actual", len(payload))
		s.sendResponse(hdr, adsErrInvalidSize, nil)
		return
	}

	data := payload[amsHeaderSize+12 : amsHeaderSize+12+int(writeLen)]
	s.log.Info("write received", "indexGroup", indexGroup, "indexOffset", indexOffset, "length", writeLen)

	if s.onWrite != nil {
		if adsErr := s.onWrite(indexGroup, indexOffset, data); adsErr != 0 {
			s.log.Warn("write: handler error", "indexGroup", indexGroup, "indexOffset", indexOffset, "adsErr", adsErr)
			s.sendResponse(hdr, adsErr, nil)
			return
		}
	}

	s.sendResponse(hdr, 0, nil)
}

func (s *Server) handleReadWrite(hdr *AmsHeader, payload []byte) {
	const reqHdrLen = 16 // IndexGroup(4) + IndexOffset(4) + ReadLength(4) + WriteLength(4)

	if len(payload) < amsHeaderSize+reqHdrLen {
		s.log.Warn("readwrite: payload too short", "len", len(payload))
		s.sendResponse(hdr, adsErrInvalidSize, nil)
		return
	}

	req := payload[amsHeaderSize:]
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	readLen := binary.LittleEndian.Uint32(req[8:12])
	writeLen := binary.LittleEndian.Uint32(req[12:16])

	if len(payload) < amsHeaderSize+reqHdrLen+int(writeLen) {
		s.log.Warn("readwrite: writeLen exceeds payload", "declared", writeLen, "actual", len(payload))
		s.sendResponse(hdr, adsErrInvalidSize, nil)
		return
	}

	writeData := payload[amsHeaderSize+reqHdrLen : amsHeaderSize+reqHdrLen+int(writeLen)]
	readData := make([]byte, readLen)

	if s.onReadWrite != nil {
		n, adsErr := s.onReadWrite(indexGroup, indexOffset, readData, writeData)
		if adsErr != 0 {
			s.log.Warn("readwrite: handler error", "indexGroup", indexGroup, "indexOffset", indexOffset, "adsErr", adsErr)
			s.sendResponse(hdr, adsErr, nil)
			return
		}
		readData = readData[:n]
	}

	// Prepend the ReadLength field then the data in one allocation.
	body := binary.LittleEndian.AppendUint32(make([]byte, 0, 4+len(readData)), uint32(len(readData)))
	body = append(body, readData...)
	s.sendResponse(hdr, 0, body)
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
func (s *Server) sendResponse(hdr *AmsHeader, adsErr uint32, payload []byte) {
	// Mutate header in place — caller does not use it after this point.
	hdr.Flags = FlagResponse | 0x0004
	hdr.Error = 0
	hdr.Length = amsErrFieldSize + uint32(len(payload))
	hdr.Target, hdr.Source = hdr.Source, hdr.Target
	hdr.TargetPort, hdr.SourcePort = hdr.SourcePort, hdr.TargetPort

	pb := bufPool.Get().(*pooledBuf)
	buf := pb.data[:0]

	// AmsTcpHeader
	buf = binary.LittleEndian.AppendUint16(buf, 0) // Reserved
	buf = binary.LittleEndian.AppendUint32(buf, uint32(amsHeaderSize+amsErrFieldSize+len(payload)))

	// AmsHeader
	buf = append(buf, hdr.Target[:]...)
	buf = binary.LittleEndian.AppendUint16(buf, hdr.TargetPort)
	buf = append(buf, hdr.Source[:]...)
	buf = binary.LittleEndian.AppendUint16(buf, hdr.SourcePort)
	buf = binary.LittleEndian.AppendUint16(buf, hdr.Command)
	buf = binary.LittleEndian.AppendUint16(buf, hdr.Flags)
	buf = binary.LittleEndian.AppendUint32(buf, hdr.Length)
	buf = binary.LittleEndian.AppendUint32(buf, hdr.Error)
	buf = binary.LittleEndian.AppendUint32(buf, hdr.InvokeID)

	// Body
	buf = binary.LittleEndian.AppendUint32(buf, adsErr)
	buf = append(buf, payload...)

	pb.data = buf

	// time.NewTimer reuses a single runtime timer object; time.After would allocate
	// a new goroutine and channel on every call.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case s.writeChan <- pb:
		// writer now owns pb and will return it to the pool.
	case <-timer.C:
		pb.data = pb.data[:0]
		bufPool.Put(pb)
		s.log.Error("sendResponse: write channel blocked, dropping packet",
			"invokeID", hdr.InvokeID,
			"cmd", hdr.Command,
			"queueLen", len(s.writeChan),
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
