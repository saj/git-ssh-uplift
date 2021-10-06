package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"io"
	"net"
)

func parseIPv4HexLittleEndian(b []byte) (net.IP, bool) {
	if len(b) != net.IPv4len*2 {
		return nil, false
	}
	ip := make(net.IP, net.IPv4len)
	for i := 0; i < net.IPv4len; i++ {
		_, err := hex.Decode(ip[net.IPv4len-i-1:], slice2b(b[i*2:]))
		if err != nil {
			return nil, false
		}
	}
	return ip, true
}

func slice2b(b []byte) []byte { return b[:2] }

var linuxDefaultDestination = []byte("00000000")

func parseLinuxGatewayIPAddr(r io.Reader) (net.IP, bool) {
	s := bufio.NewScanner(r)
	if !s.Scan() { // skip header
		return nil, false
	}
	for s.Scan() {
		f := bytes.Fields(s.Bytes())
		if len(f) < 3 {
			continue
		}
		dest := f[1]
		gate := f[2]
		if !bytes.Equal(dest, linuxDefaultDestination) {
			continue
		}

		ip, ok := parseIPv4HexLittleEndian(gate)
		if !ok {
			continue
		}
		return ip, true
	}
	return nil, false
}
