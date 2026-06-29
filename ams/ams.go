package ams

const (
	TcpHeaderSize = 6
	HeaderSize    = 32
)

type NetId [6]byte

type Address struct {
	NetId NetId
	Port  uint16
}
