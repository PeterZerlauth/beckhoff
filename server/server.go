package server

import (
	"encoding/binary"
	"log/slog"
	"sync"

	"github.com/PeterZerlauth/beckhoff/ads"
	"github.com/PeterZerlauth/beckhoff/ams"
)

/* ===================== SERVER ===================== */

type Server struct {
	conn *ams.Connection
	name string

	mu sync.RWMutex

	log    *slog.Logger
	logger *Logger

	// ✅ custom hooks
	OnRead      func(ig, io uint32, buf []byte) ads.ErrorCode
	OnWrite     func(ig, io uint32, data []byte) ads.ErrorCode
	OnReadWrite func(ig, io uint32, readBuf []byte, writeData []byte) ads.ErrorCode
}

/* ===================== CONSTRUCTOR ===================== */

func NewServer(port uint16, name string) *Server {
	logger := NewLogger("logger.log", 5)

	s := &Server{
		name:   name,
		log:    logger.Slog(),
		logger: logger,
	}

	s.conn = ams.NewConnection(port, s, s.log)

	return s
}

/* ===================== LIFECYCLE ===================== */

func (s *Server) Start() error {
	if err := s.conn.Start(); err != nil {
		s.log.Error("server start failed", "error", err)
		return err
	}

	s.log.Info("server started", "netid", s.conn.NetID(), "port", s.conn.Port())
	return nil
}

func (s *Server) NetID() string {
	return s.conn.NetID()
}

func (s *Server) Close() {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.log != nil {
		s.log.Info("server shutting down")
	}
	if s.logger != nil {
		s.logger.Close()
	}
}

/* ===================== HANDLER ===================== */

func (s *Server) HandlePacket(amsPackage []byte) ([]byte, error) {

	cmd := binary.LittleEndian.Uint16(amsPackage[16:18])
	invoke := binary.LittleEndian.Uint32(amsPackage[28:32])
	request := amsPackage[ams.HeaderSize:]

	switch cmd {

	case ads.CmdReadDeviceInfo:
		return s.buildReadDeviceInfo(amsPackage, invoke, s.name, 1, 2, 3), nil

	case ads.CmdReadState:
		return s.buildReadState(amsPackage, invoke)

	case ads.CmdRead:
		return s.handleRead(amsPackage, request, invoke)

	case ads.CmdWrite:
		return s.handleWrite(amsPackage, request, invoke)

	case ads.CmdReadWrite:
		return s.handleReadWrite(amsPackage, request, invoke)
	}

	return nil, nil
}

/* ===================== COMMANDS ===================== */

func (s *Server) handleRead(p []byte, req []byte, invoke uint32) ([]byte, error) {
	ig := binary.LittleEndian.Uint32(req[0:4])
	io := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	buf := make([]byte, length)

	s.mu.RLock()
	var err ads.ErrorCode
	if s.OnRead != nil {
		err = s.OnRead(ig, io, buf)
	} else {
		s.log.Error("OnRead not implemented")
		err = ads.InvalidIndexOffset
	}
	s.mu.RUnlock()

	return buildReadResponse(p, invoke, err, buf), nil
}

func (s *Server) handleWrite(p []byte, req []byte, invoke uint32) ([]byte, error) {
	ig := binary.LittleEndian.Uint32(req[0:4])
	io := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	data := req[12 : 12+length]

	s.mu.Lock()
	var err ads.ErrorCode
	if s.OnWrite != nil {
		err = s.OnWrite(ig, io, data)
	} else {
		s.log.Error("OnWrite not implemented")
		err = ads.InvalidIndexOffset
	}
	s.mu.Unlock()

	return buildWriteResponse(p, invoke, err), nil
}

func (s *Server) handleReadWrite(p []byte, req []byte, invoke uint32) ([]byte, error) {
	ig := binary.LittleEndian.Uint32(req[0:4])
	io := binary.LittleEndian.Uint32(req[4:8])
	readLen := binary.LittleEndian.Uint32(req[8:12])
	writeLen := binary.LittleEndian.Uint32(req[12:16])

	if int(16+writeLen) > len(req) {
		return buildReadWriteResponse(p, invoke, ads.InvalidParameter, nil), nil
	}

	writeData := req[16 : 16+writeLen]
	readBuf := make([]byte, readLen)

	s.mu.Lock()
	var err ads.ErrorCode
	if s.OnReadWrite != nil {
		err = s.OnReadWrite(ig, io, readBuf, writeData)
	} else {
		s.log.Error("OnReadWrite not implemented")
		err = ads.InvalidIndexOffset
	}
	s.mu.Unlock()

	return buildReadWriteResponse(p, invoke, err, readBuf), nil
}
