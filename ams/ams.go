package ams

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

const (
	TcpHeaderSize = 6
	HeaderSize    = 32
)

type NetId [6]byte

type Address struct {
	NetId NetId
	Port  uint16
}

// NetID to String
func (n *NetId) FromBytes(netId []byte) {
	*n = NetId(netId)
}

// NetID to String
func (n NetId) String() string {
	return fmt.Sprintf("%d.%d.%d.%d.%d.%d",
		n[0], n[1], n[2], n[3], n[4], n[5])
}

// Parse NetId
func ParseNetId(s string) (NetId, error) {
	var n NetId

	parts := strings.Split(s, ".")
	if len(parts) != 6 {
		return n, fmt.Errorf("invalid NetId format")
	}

	for i := 0; i < 6; i++ {
		v, err := strconv.Atoi(parts[i])
		if err != nil || v < 0 || v > 255 {
			return n, fmt.Errorf("invalid value: %s", parts[i])
		}
		n[i] = byte(v)
	}

	return n, nil
}

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
