package server

import (
	"bytes"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/peterzerlauth/beckhoff/ads"
	"github.com/peterzerlauth/beckhoff/ams"
)

type Server struct {
	conn  net.Conn
	port  uint16
	netid ams.NetId

	mem map[[2]uint32][]byte
	mu  sync.RWMutex

	wmu sync.Mutex

	jobs chan []byte
	log  *slog.Logger

	symbol *SymbolTable
}

var bufPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 1024)
	},
}

func New(port uint16) *Server {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
	}))

	return &Server{
		port:   port,
		mem:    make(map[[2]uint32][]byte),
		jobs:   make(chan []byte, 1024),
		log:    logger,
		symbol: NewSymbolTable(),
	}
}

func (s *Server) Start() error {
	conn, err := net.Dial("tcp", "127.0.0.1:48898")
	if err != nil {
		s.log.Error("Ads server", "err", err)
		return err
	}
	s.conn = conn

	if err := s.register(); err != nil {
		s.log.Error("Ads server", "err", err)
		return err
	}

	s.log.Info("Ads server started", "NetID", s.netid, "Port", s.port)

	for i := 0; i < 4; i++ {
		go s.worker()
	}

	go s.loop()
	return nil
}

func (s *Server) register() error {
	var req [8]byte
	req[1] = 16
	req[2] = 2
	binary.LittleEndian.PutUint16(req[6:], s.port)

	if _, err := s.conn.Write(req[:]); err != nil {
		return err
	}

	var res [14]byte
	if _, err := io.ReadFull(s.conn, res[:]); err != nil {
		return err
	}

	copy(s.netid[:], res[6:12])
	s.port = binary.LittleEndian.Uint16(res[12:14])

	s.log.Debug("Ads server registered")
	return nil
}

func (s *Server) loop() {
	for {
		p, err := s.readPacket()
		if err != nil {
			s.log.Error("connection closed", "err", err)
			return
		}
		s.jobs <- p
	}
}

func (s *Server) worker() {
	for p := range s.jobs {
		s.handle(p)
		bufPool.Put(p[:0])
	}
}

func (s *Server) readPacket() ([]byte, error) {
	var tcp [ads.TcpHeaderSize]byte

	if _, err := io.ReadFull(s.conn, tcp[:]); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint32(tcp[2:])
	buf := bufPool.Get().([]byte)

	if cap(buf) < int(length) {
		buf = make([]byte, length)
	}
	buf = buf[:length]

	if _, err := io.ReadFull(s.conn, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

func (s *Server) handle(p []byte) {
	cmd := binary.LittleEndian.Uint16(p[16:18])
	invoke := binary.LittleEndian.Uint32(p[28:32])
	request := p[ams.HeaderSize:]

	switch cmd {

	case ads.CmdReadDeviceInfo:
		s.log.Debug("CmdReadDeviceInfo")
		s.sendDeviceInfo(p, invoke)

	case ads.CmdRead:
		ig := binary.LittleEndian.Uint32(request[0:4])
		io := binary.LittleEndian.Uint32(request[4:8])
		length := binary.LittleEndian.Uint32(request[8:12])

		s.log.Debug("Ads read",
			"IndexGroup", ig,
			"IndexOffset", io,
			"Length", length,
		)

		data, err := s.Read(ig, io, length)
		s.sendReadResponse(p, invoke, err, data)

	case ads.CmdWrite:
		ig := binary.LittleEndian.Uint32(request[0:4])
		io := binary.LittleEndian.Uint32(request[4:8])
		length := binary.LittleEndian.Uint32(request[8:12])
		data := request[12 : 12+length]

		s.log.Debug("Ads write",
			"IndexGroup", ig,
			"IndexOffset", io,
			"Length", length,
		)

		err := s.Write(ig, io, append([]byte{}, data...))

		body := make([]byte, 4)
		binary.LittleEndian.PutUint32(body, uint32(err))
		s.sendRaw(p, ads.CmdWrite, invoke, 4, body)

	case ads.CmdReadWrite:
		ig := binary.LittleEndian.Uint32(request[0:4])
		io := binary.LittleEndian.Uint32(request[4:8])
		readLen := binary.LittleEndian.Uint32(request[8:12])
		writeLen := binary.LittleEndian.Uint32(request[12:16])
		writeData := request[16 : 16+writeLen]

		s.log.Debug("Ads read/write",
			"IndexGroup", ig,
			"IndexOffset", io,
			"ReadLen", readLen,
			"WriteLen", writeLen,
		)

		// Get handle by name
		if ig == 0xF003 {
			name := string(bytes.TrimRight(writeData, "\x00"))

			handle, errCode := s.symbol.GetHandle(name)

			if errCode != ads.NoError {
				s.log.Warn("Ads symbol not found", "name", name)

				s.sendReadWriteResponse(p, invoke, errCode, nil)
				return
			}

			s.log.Debug("Ads get handle", "name", name, "handle", handle)
			s.sendHandleResponse(p, invoke, handle)

			return
		}

		data, err := s.ReadWrite(ig, io, readLen, writeData)
		s.sendReadWriteResponse(p, invoke, err, data)
	}
}

/* ===================== CORE ===================== */
func (s *Server) Write(indexGroup, indexOffset uint32, data []byte) ads.ErrorCode {

	s.log.Debug("Ads Write",
		"IndexGroup", indexGroup,
		"IndexOffset", indexOffset,
	)

	// ✅ Symbol write (ADS handles)
	if indexGroup == 0xF005 || indexGroup == 0xF006 {

		name, ok := s.symbol.Name(indexOffset)
		if ok {
			s.log.Debug("Ads Write Symbol",
				"handle", indexOffset,
				"symbol", name,
				"size", len(data),
			)
		} else {
			s.log.Warn("Ads unknown handle", "handle", indexOffset)
			return ads.SymbolNotFound
		}

		return s.symbol.Write(indexOffset, data)
	}

	// ✅ fallback
	return s.OnWrite(indexGroup, indexOffset, data)
}

func (s *Server) Read(indexGroup, indexOffset, length uint32) ([]byte, ads.ErrorCode) {
	if indexGroup == 0xF005 {
		name, ok := s.symbol.Name(indexOffset)
		if ok {
			s.log.Debug("Read symbol",
				"symbol", name,
				"handle", indexOffset,
			)
		}

		data, err := s.symbol.Read(indexOffset)
		if err != ads.NoError {
			s.log.Warn("ads symbol not found", "handle", indexOffset)
			return nil, err
		}

		if int(length) < len(data) {
			data = data[:length]
		}
		return data, ads.NoError
	}

	return s.OnRead(indexGroup, indexOffset, length)
}

func (s *Server) ReadWrite(indexGroup, indexOffset, length uint32, writeData []byte) ([]byte, ads.ErrorCode) {
	switch indexGroup {

	case 0xF005:
		handle := binary.LittleEndian.Uint32(writeData)
		s.log.Debug("Ads read write", "handle", handle)
		return s.symbol.Read(handle)

	case 0xF006:
		handle := binary.LittleEndian.Uint32(writeData[:4])
		s.log.Debug("Ads read write", "handle", handle)

		if len(writeData) > 4 {
			return nil, s.symbol.Write(handle, writeData[4:])
		}
		return nil, ads.NoError
	}

	return s.OnReadWrite(indexGroup, indexOffset, length, writeData)
}

func (s *Server) OnRead(indexGroup, indexOffset, length uint32) ([]byte, ads.ErrorCode) {
	s.mu.RLock()
	data := s.mem[[2]uint32{indexGroup, indexOffset}]
	s.mu.RUnlock()

	s.log.Debug("function OnRead",
		"IndexGroup", indexGroup,
		"IndexOffset", indexOffset,
	)

	if len(data) == 0 {
		return make([]byte, length), ads.NoError
	}

	if int(length) < len(data) {
		return data[:length], ads.NoError
	}

	return data, ads.NoError
}

func (s *Server) OnWrite(indexGroup, indexOffset uint32, data []byte) ads.ErrorCode {
	s.log.Debug("OnWrite",
		"IndexGroup", indexGroup,
		"IndexOffset", indexOffset,
		"Size", len(data),
	)

	// store into memory (generic ADS memory area)
	cp := append([]byte{}, data...)

	s.mu.Lock()
	s.mem[[2]uint32{indexGroup, indexOffset}] = cp
	s.mu.Unlock()

	return ads.NoError
}

func (s *Server) OnReadWrite(indexGroup, indexOffset, readLen uint32, writeData []byte) ([]byte, ads.ErrorCode) {
	s.log.Debug("function OnReadWrite",
		"IndexGroup", indexGroup,
		"IndexOffset", indexOffset,
		"ReadLen", readLen,
		"WriteLen", len(writeData),
	)

	// WRITE phase (if any)
	if len(writeData) > 0 {
		cp := append([]byte{}, writeData...) // avoid buffer reuse issues

		s.mu.Lock()
		s.mem[[2]uint32{indexGroup, indexOffset}] = cp
		s.mu.Unlock()
	}

	// READ phase
	s.mu.RLock()
	data := s.mem[[2]uint32{indexGroup, indexOffset}]
	s.mu.RUnlock()

	if int(readLen) < len(data) {
		return data[:readLen], ads.NoError
	}

	return data, ads.NoError
}

/* ===================== RESPONSES ===================== */

func (s *Server) sendHandleResponse(req []byte, invoke uint32, handle uint32) {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[0:4], uint32(ads.NoError))
	binary.LittleEndian.PutUint32(body[4:8], handle)
	s.sendRaw(req, ads.CmdReadWrite, invoke, 8, body)
}

func (s *Server) sendReadResponse(req []byte, invoke uint32, err ads.ErrorCode, data []byte) {
	body := make([]byte, 8+len(data))
	binary.LittleEndian.PutUint32(body[0:4], uint32(err))
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(data)))
	copy(body[8:], data)
	s.sendRaw(req, ads.CmdRead, invoke, uint32(len(body)), body)
}

func (s *Server) sendReadWriteResponse(req []byte, invoke uint32, err ads.ErrorCode, data []byte) {
	body := make([]byte, 8+len(data))
	binary.LittleEndian.PutUint32(body[0:4], uint32(err))
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(data)))
	copy(body[8:], data)
	s.sendRaw(req, ads.CmdReadWrite, invoke, uint32(len(body)), body)
}

func (s *Server) sendDeviceInfo(req []byte, invoke uint32) {
	body := make([]byte, 24)
	binary.LittleEndian.PutUint32(body[0:4], uint32(ads.NoError))
	body[4] = 1
	body[5] = 0
	binary.LittleEndian.PutUint16(body[6:8], 1)
	copy(body[8:], []byte("Go ADS Server"))
	s.sendRaw(req, ads.CmdReadDeviceInfo, invoke, uint32(len(body)), body)
}

/* ===================== SEND ===================== */

func (s *Server) sendRaw(req []byte, cmd uint16, invoke uint32, dataLen uint32, payload []byte) {
	total := ams.HeaderSize + dataLen

	buf := make([]byte, 0, ads.TcpHeaderSize+total)
	buf = binary.LittleEndian.AppendUint16(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, total)

	buf = append(buf, req[8:14]...)
	buf = binary.LittleEndian.AppendUint16(buf, binary.LittleEndian.Uint16(req[14:16]))

	buf = append(buf, req[0:6]...)
	buf = binary.LittleEndian.AppendUint16(buf, binary.LittleEndian.Uint16(req[6:8]))

	buf = binary.LittleEndian.AppendUint16(buf, cmd)
	buf = binary.LittleEndian.AppendUint16(buf, 0x0005)
	buf = binary.LittleEndian.AppendUint32(buf, dataLen)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, invoke)

	buf = append(buf, payload...)

	s.wmu.Lock()
	_, _ = s.conn.Write(buf)
	s.wmu.Unlock()
}

/* ===================== UTIL ===================== */

func (s *Server) NetID() string {
	return s.netid.String()
}

func (s *Server) Symbol() *SymbolTable {
	return s.symbol
}
