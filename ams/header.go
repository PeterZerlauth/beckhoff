package ams

import (
	"encoding/binary"
	"fmt"
)

// AMS Header
type Header struct {
	TargetNetId NetId
	TargetPort  uint16

	SourceNetId NetId
	SourcePort  uint16

	CommandID  uint16
	StateFlags uint16

	DataLength uint32
	ErrorCode  uint32
	InvokeID   uint32
}

func (h *Header) Encode() []byte {
	var buf [32]byte

	copy(buf[0:6], h.TargetNetId[:])
	binary.LittleEndian.PutUint16(buf[6:8], h.TargetPort)

	copy(buf[8:14], h.SourceNetId[:])
	binary.LittleEndian.PutUint16(buf[14:16], h.SourcePort)

	binary.LittleEndian.PutUint16(buf[16:18], h.CommandID)
	binary.LittleEndian.PutUint16(buf[18:20], h.StateFlags)
	binary.LittleEndian.PutUint32(buf[20:24], h.DataLength)
	binary.LittleEndian.PutUint32(buf[24:28], h.ErrorCode)
	binary.LittleEndian.PutUint32(buf[28:32], h.InvokeID)

	return buf[:]
}

func Decode(data []byte) (*Header, error) {
	if len(data) < 32 {
		return nil, fmt.Errorf("invalid AMS header size: %d", len(data))
	}

	h := &Header{}

	copy(h.TargetNetId[:], data[0:6])
	h.TargetPort = binary.LittleEndian.Uint16(data[6:8])

	copy(h.SourceNetId[:], data[8:14])
	h.SourcePort = binary.LittleEndian.Uint16(data[14:16])

	h.CommandID = binary.LittleEndian.Uint16(data[16:18])
	h.StateFlags = binary.LittleEndian.Uint16(data[18:20])
	h.DataLength = binary.LittleEndian.Uint32(data[20:24])
	h.ErrorCode = binary.LittleEndian.Uint32(data[24:28])
	h.InvokeID = binary.LittleEndian.Uint32(data[28:32])

	return h, nil
}
