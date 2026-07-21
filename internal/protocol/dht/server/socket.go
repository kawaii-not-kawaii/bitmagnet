package server

import (
	"net/netip"
)

type Socket interface {
	Open(localAddr netip.AddrPort) error
	Close() error
	Send(netip.AddrPort, []byte) error
	Receive([]byte) (int, netip.AddrPort, error)
}

func NewSocket(ipType int) Socket {
	return newSocket(ipType)
}
