package main

import (
	"fmt"
	"net"
	"os"
)

func dialByGatewayAddress() (net.Conn, error) {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil, dialMethodUnavailableError
	}
	defer f.Close()

	ip, ok := parseLinuxGatewayIPAddr(f)
	if !ok {
		return nil, dialMethodUnavailableError
	}

	addr := fmt.Sprintf("%s:%d", ip.String(), port())
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, dialMethodUnavailableError
	}
	return conn, err
}
