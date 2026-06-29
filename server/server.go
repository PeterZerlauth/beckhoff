package server

import (
	"encoding/binary"
	"log/slog"
	"sync"

	"github.com/PeterZerlauth/beckhoff/ads"
	"github.com/PeterZerlauth/beckhoff/ams"
	"github.com/PeterZerlauth/beckhoff/logger"
)

type Server struct {
	conn *ams.Connection
	name string

	mu sync.RWMutex

	log *slog.Logger

	OnRead      func(uint32, uint32, []byte) ads.ErrorCode
	OnWrite     func(uint32, uint32, []byte) ads.ErrorCode
	OnReadWrite func(uint32, uint32, []byte, []byte) ads.ErrorCode
}

func NewServer(port uint16, name string) *Server {
	log := logger.GetLogger("", 7).Log()

	s := &Server{
		name: name,
		log:  log,
	}

	s.conn = ams.NewConnection(port, s, s.log)
	return s
}

func (s *Server) Start() error {
	if err := s.conn.Start(); err != nil {
		s.log.Error("Ads server start failed", "error", err)
		return err
	}

	s.log.Info("Ads server started", "netid", s.conn.NetID(), "port", s.conn.Port())
	return nil
}

func (s *Server) Close() {
	s.log.Info("Ads server close")
	if s.conn != nil {
		s.conn.Close()
	}
	logger.GetLogger("", 7).Close()
}

/* ===================== PACKET HANDLER ===================== */

func (s *Server) HandlePacket(amsPackage []byte) ([]byte, error) {

	if len(amsPackage) < ams.HeaderSize {
		s.log.Error("AMS package too small", "len", len(amsPackage))
		return nil, nil
	}

	header, err := ams.Decode(amsPackage)
	if err != nil {
		s.log.Error("Ams Packet decoding failed", "error", err)
		return nil, err
	}

	req := amsPackage[ams.HeaderSize:]

	switch header.CommandID {

	case ads.CmdReadDeviceInfo:
		return s.buildReadDeviceInfo(header, s.name, 1, 2, 3), nil

	case ads.CmdReadState:
		return s.buildReadState(header), nil

	case ads.CmdRead:
		return s.handleRead(header, req), nil

	case ads.CmdWrite:
		return s.handleWrite(header, req), nil

	case ads.CmdReadWrite:
		return s.handleReadWrite(header, req), nil
	}

	s.log.Warn("unknown command", "cmd", header.CommandID)
	return nil, nil
}

/* ===================== COMMAND HANDLERS ===================== */

func (s *Server) handleRead(header *ams.Header, req []byte) []byte {
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	data := make([]byte, length)

	s.mu.RLock()
	var err ads.ErrorCode
	if s.OnRead != nil {
		err = s.OnRead(indexGroup, indexOffset, data)
	} else {
		err = ads.InvalidIndexOffset
		s.log.Error("OnRead not implemented")
	}
	s.mu.RUnlock()

	return buildReadResponse(header, err, data)
}

func (s *Server) handleWrite(header *ams.Header, req []byte) []byte {
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	length := binary.LittleEndian.Uint32(req[8:12])

	data := req[12 : 12+length]

	s.mu.Lock()
	var err ads.ErrorCode
	if s.OnWrite != nil {
		err = s.OnWrite(indexGroup, indexOffset, data)
	} else {
		err = ads.InvalidIndexOffset
		s.log.Error("OnWrite not implemented")
	}
	s.mu.Unlock()

	return buildWriteResponse(header, err)
}

func (s *Server) handleReadWrite(header *ams.Header, req []byte) []byte {
	indexGroup := binary.LittleEndian.Uint32(req[0:4])
	indexOffset := binary.LittleEndian.Uint32(req[4:8])
	readLen := binary.LittleEndian.Uint32(req[8:12])
	writeLen := binary.LittleEndian.Uint32(req[12:16])

	if int(16+writeLen) > len(req) {
		return buildReadWriteResponse(header, ads.InvalidParameter, nil)
	}

	writeData := req[16 : 16+writeLen]
	readData := make([]byte, readLen)

	s.mu.Lock()
	var err ads.ErrorCode
	if s.OnReadWrite != nil {
		err = s.OnReadWrite(indexGroup, indexOffset, readData, writeData)
	} else {
		err = ads.InvalidIndexOffset
		s.log.Error("OnReadWrite not implemented")
	}
	s.mu.Unlock()

	return buildReadWriteResponse(header, err, readData)
}
