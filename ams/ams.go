package ams

import (
	"bytes"
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

func (h *Header) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)

	err := binary.Write(buf, binary.LittleEndian, h)
	if err != nil {
		return nil, fmt.Errorf("encode AMS header failed: %w", err)
	}

	return buf.Bytes(), nil
}

func Decode(data []byte) (*Header, error) {
	if len(data) < 32 {
		return nil, fmt.Errorf("invalid AMS header size: %d", len(data))
	}

	h := &Header{}

	buf := bytes.NewReader(data[:32])

	err := binary.Read(buf, binary.LittleEndian, h)
	if err != nil {
		return nil, fmt.Errorf("decode AMS header failed: %w", err)
	}

	return h, nil
}
