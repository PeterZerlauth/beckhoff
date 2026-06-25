package server

import (
	"encoding/binary"

	"github.com/PeterZerlauth/beckhoff/ads"
	"github.com/PeterZerlauth/beckhoff/ams"
)

/* ===================== PUBLIC BUILDERS ===================== */

func buildReadResponse(req []byte, invoke uint32, err ads.ErrorCode, data []byte) []byte {
	body := make([]byte, 8+len(data))
	binary.LittleEndian.PutUint32(body[0:4], uint32(err))
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(data)))
	copy(body[8:], data)

	return buildAms(req, ads.CmdRead, invoke, body)
}

func buildWriteResponse(req []byte, invoke uint32, err ads.ErrorCode) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body, uint32(err))

	return buildAms(req, ads.CmdWrite, invoke, body)
}

func buildReadWriteResponse(req []byte, invoke uint32, err ads.ErrorCode, data []byte) []byte {
	body := make([]byte, 8+len(data))
	binary.LittleEndian.PutUint32(body[0:4], uint32(err))
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(data)))
	copy(body[8:], data)

	return buildAms(req, ads.CmdReadWrite, invoke, body)
}

func (s *Server) buildReadDeviceInfo(req []byte, invoke uint32, name string, major byte, minor byte, build uint16) []byte {

	var body [24]byte

	// Result
	binary.LittleEndian.PutUint32(body[0:4], uint32(ads.NoError))

	// Version (major, minor, build)
	body[4] = major
	body[5] = minor
	//	binary.LittleEndian.PutUint16(body[6:8], build)
	binary.LittleEndian.PutUint16(body[6:8], build)

	// Device name (max 16 bytes)
	nameBytes := []byte(name)
	if len(nameBytes) > 16 {
		nameBytes = nameBytes[:16]
	}
	copy(body[8:24], nameBytes)

	s.log.Debug("ReadDeviceInfo", "name", nameBytes, "major", major, "minor", minor, "build", build)

	return buildAms(req, ads.CmdReadDeviceInfo, invoke, body[:])
}
func (s *Server) buildReadState(req []byte, invoke uint32) ([]byte, error) {
	var body [8]byte

	// Result
	binary.LittleEndian.PutUint32(body[0:4], uint32(ads.NoError))

	// ADS State (RUN)
	binary.LittleEndian.PutUint16(body[4:6], uint16(ads.STATE_RUN))

	// Device State
	binary.LittleEndian.PutUint16(body[6:8], 0)

	s.log.Debug("ReadState", "ADS State", ads.STATE_RUN, "Device State", 0)

	return buildAms(req, ads.CmdReadState, invoke, body[:]), nil
}

/* ===================== CORE AMS BUILDER ===================== */

func buildAms(req []byte, cmd uint16, invoke uint32, payload []byte) []byte {
	dataLen := uint32(len(payload))
	total := ams.HeaderSize + dataLen

	buf := make([]byte, 0, ads.TcpHeaderSize+total)

	// TCP header
	buf = binary.LittleEndian.AppendUint16(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, total)

	// AMS header (swap source/target from request)

	// Target = Source from request
	buf = append(buf, req[8:14]...)
	buf = binary.LittleEndian.AppendUint16(buf, binary.LittleEndian.Uint16(req[14:16]))

	// Source = Target from request
	buf = append(buf, req[0:6]...)
	buf = binary.LittleEndian.AppendUint16(buf, binary.LittleEndian.Uint16(req[6:8]))

	// Command
	buf = binary.LittleEndian.AppendUint16(buf, cmd)

	// State flags (response)
	buf = binary.LittleEndian.AppendUint16(buf, 0x0005)

	// Data length
	buf = binary.LittleEndian.AppendUint32(buf, dataLen)

	// Error code
	buf = binary.LittleEndian.AppendUint32(buf, 0)

	// Invoke ID
	buf = binary.LittleEndian.AppendUint32(buf, invoke)

	// Payload
	buf = append(buf, payload...)

	return buf
}
