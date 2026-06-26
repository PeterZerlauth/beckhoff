package server

import (
	"encoding/binary"
	"log/slog"
	"sync"

	"github.com/PeterZerlauth/beckhoff/ads"
	"github.com/PeterZerlauth/beckhoff/ams"
	"github.com/PeterZerlauth/beckhoff/logger"
)

/* Beckhoff ads server */

type Server struct {
	conn *ams.Connection
	name string

	mu sync.RWMutex

	log    *slog.Logger
	logger *logger.Logger

	// ads commands
	OnRead      func(indexGroup, indexOffset uint32, readData []byte) ads.ErrorCode
	OnWrite     func(indexGroup, indexOffset uint32, dataData []byte) ads.ErrorCode
	OnReadWrite func(indexGroup, indexOffset uint32, readData []byte, writeData []byte) ads.ErrorCode
}

/* Create new server */

func NewServer(port uint16, name string) *Server {
	logger := logger.NewLogger("logger.log", 5)

	s := &Server{
		name:   name,
		log:    logger.Slog(),
		logger: logger,
	}

	s.conn = ams.NewConnection(port, s, s.log)

	return s
}

/* Start server */

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


/* Close server */
func (s *Server) Close() {

	if s.conn != nil {
		s.conn.Close()
	}
	if s.log != nil {
		s.log.Info("server close")
	}
	if s.logger != nil {
		s.logger.Close()
	}
}

/* Handle ads Packets */

func (s *Server) HandlePacket(amsPackage []byte) ([]byte, error) {

	command := binary.LittleEndian.Uint16(amsPackage[16:18])
	invoke := binary.LittleEndian.Uint32(amsPackage[28:32])
	request := amsPackage[ams.HeaderSize:]

	switch command {

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

/* Beckhoff ads commands */

func (s *Server) handleRead(p []byte, req []byte, invoke uint32) ([]byte, error) {
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	readData := make([]byte, length)

	s.mu.RLock()
	var err ads.ErrorCode
	if s.OnRead != nil {
		err = s.OnRead(indexGroup, indexOffset, readData)
	} else {
		s.log.Error("OnRead not implemented")
		err = ads.InvalidIndexOffset
	}
	s.mu.RUnlock()

	return buildReadResponse(p, invoke, err, readData), nil
}

func (s *Server) handleWrite(p []byte, req []byte, invoke uint32) ([]byte, error) {
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	writeData := req[12 : 12+length]

	s.mu.Lock()
	var err ads.ErrorCode
	if s.OnWrite != nil {
		err = s.OnWrite(indexGroup, indexOffset, writeData)
	} else {
		s.log.Error("OnWrite not implemented")
		err = ads.InvalidIndexOffset
	}
	s.mu.Unlock()

	return buildWriteResponse(p, invoke, err), nil
}

func (s *Server) handleReadWrite(p []byte, req []byte, invoke uint32) ([]byte, error) {
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	readLen := binary.LittleEndian.Uint32(req[8:12])
	writeLen := binary.LittleEndian.Uint32(req[12:16])

	if int(16+writeLen) > len(req) {
		return buildReadWriteResponse(p, invoke, ads.InvalidParameter, nil), nil
	}

	writeData := req[16 : 16+writeLen]
	readData := make([]byte, readLen)

	s.mu.Lock()
	var err ads.ErrorCode
	if s.OnReadWrite != nil {
		err = s.OnReadWrite(indexGroup, indexOffset, readData, writeData)
	} else {
		s.log.Error("OnReadWrite not implemented")
		err = ads.InvalidIndexOffset
	}
	s.mu.Unlock()

	return buildReadWriteResponse(p, invoke, err, readData), nil
}

func (s *Server) Log() *slog.Logger {
	return s.log
}
