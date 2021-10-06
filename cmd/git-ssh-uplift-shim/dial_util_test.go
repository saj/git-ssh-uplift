package main

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestParseHexIPv4Zero(t *testing.T) {
	ip, ok := parseIPv4HexLittleEndian([]byte("00000000"))
	assert.Assert(t, ok)
	assert.Equal(t, ip.String(), "0.0.0.0")
}

func TestParseHexIPv4NonZero(t *testing.T) {
	ip, ok := parseIPv4HexLittleEndian([]byte("010011AC"))
	assert.Assert(t, ok)
	assert.Equal(t, ip.String(), "172.17.0.1")
}

func TestParseLinuxGatewayIPAddr(t *testing.T) {
	f := strings.NewReader(`Iface   Destination     Gateway         Flags   RefCnt  Use     Metric  Mask            MTU     Window  IRTT
eth0    00000000        010011AC        0003    0       0       0       00000000        0       0       0
eth0    000011AC        00000000        0001    0       0       0       0000FFFF        0       0       0
`)
	ip, ok := parseLinuxGatewayIPAddr(f)
	assert.Assert(t, ok)
	assert.Equal(t, ip.String(), "172.17.0.1")
}
