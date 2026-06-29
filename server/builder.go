package server

import (
	"encoding/binary"

	"github.com/PeterZerlauth/beckhoff/ads"
	"github.com/PeterZerlauth/beckhoff/ams"
)

/* ===================== RESPONSE BUILDERS ===================== */

func buildReadResponse(req *ams.Header, err ads.ErrorCode, data []byte) []byte {
	body := make([]byte, 8+len(data))

	binary.LittleEndian.PutUint32(body[0:4], uint32(err))
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(data)))
	copy(body[8:], data)

	return buildAms(req, ads.CmdRead, body)
}

func buildWriteResponse(req *ams.Header, err ads.ErrorCode) []byte {
	var body [4]byte
	binary.LittleEndian.PutUint32(body[:], uint32(err))

	return buildAms(req, ads.CmdWrite, body[:])
}

func buildReadWriteResponse(req *ams.Header, err ads.ErrorCode, data []byte) []byte {
	body := make([]byte, 8+len(data))

	binary.LittleEndian.PutUint32(body[0:4], uint32(err))
	binary.LittleEndian.PutUint32(body[4:8], uint32(len(data)))
	copy(body[8:], data)

	return buildAms(req, ads.CmdReadWrite, body)
}

/* ===================== DEVICE INFO ===================== */

func (s *Server) buildReadDeviceInfo(h *ams.Header, name string, major, minor byte, build uint16) []byte {

	var body [24]byte

	binary.LittleEndian.PutUint32(body[0:4], uint32(ads.NoError))
	body[4] = major
	body[5] = minor
	binary.LittleEndian.PutUint16(body[6:8], build)

	n := []byte(name)
	if len(n) > 16 {
		n = n[:16]
	}
	copy(body[8:], n)

	return buildAms(h, ads.CmdReadDeviceInfo, body[:])
}

func (s *Server) buildReadState(h *ams.Header) []byte {
	var body [8]byte

	binary.LittleEndian.PutUint32(body[0:4], uint32(ads.NoError))
	binary.LittleEndian.PutUint16(body[4:6], uint16(ads.STATE_RUN))
	binary.LittleEndian.PutUint16(body[6:8], 0)

	return buildAms(h, ads.CmdReadState, body[:])
}

/* ===================== CORE AMS BUILDER ===================== */

func buildAms(req *ams.Header, cmd uint16, payload []byte) []byte {

	resp := &ams.Header{
		TargetNetId: req.SourceNetId,
		TargetPort:  req.SourcePort,

		SourceNetId: req.TargetNetId,
		SourcePort:  req.TargetPort,

		CommandID:  cmd,
		StateFlags: 0x0001,

		DataLength: uint32(len(payload)),
		ErrorCode:  0,
		InvokeID:   req.InvokeID,
	}

	headerBytes, _ := resp.Encode()

	// TCP header (6 bytes)
	out := make([]byte, 0, ams.TcpHeaderSize+len(headerBytes)+len(payload))

	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(headerBytes)+len(payload)))

	out = append(out, headerBytes...)
	out = append(out, payload...)

	return out
}
