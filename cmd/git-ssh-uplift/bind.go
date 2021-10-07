//go:build !linux
// +build !linux

package main

import "net"

var defaultBindAddress = net.IPv4(127, 0, 0, 1)
