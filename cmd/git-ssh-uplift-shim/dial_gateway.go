//go:build !linux
// +build !linux

package main

import "net"

func dialByGatewayAddress() (net.Conn, error) {
	return nil, dialMethodUnavailableError
}
