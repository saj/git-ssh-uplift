package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

const dialTimeout = 3 * time.Second

var (
	dialMethodUnavailableError  = errors.New("dial method unavailable")
	noAvailableDialMethodsError = errors.New("unable to autodetect proxy address - try again with UPLIFT_HOST")
)

type dialMethod func() (net.Conn, error)

// see https://github.com/moby/moby/pull/40007 for related discussion

var dialMethods = []dialMethod{
	dialByUpliftHost,     // manual UPLIFT_HOST override
	dialByGatewayAddress, // auto locate host on Docker Engine
	dialByDockerInternal, // auto locate host on Docker Desktop
}

func dial() (net.Conn, error) {
	for _, meth := range dialMethods {
		c, err := meth()
		if errors.Is(err, dialMethodUnavailableError) {
			continue
		}
		return c, err
	}
	return nil, noAvailableDialMethodsError
}

func dialByUpliftHost() (net.Conn, error) {
	host := os.Getenv("UPLIFT_HOST")
	if host == "" {
		return nil, dialMethodUnavailableError
	}
	addr := fmt.Sprintf("%s:%d", host, port())
	return net.DialTimeout("tcp", addr, dialTimeout)
}

func dialByDockerInternal() (net.Conn, error) {
	addr := fmt.Sprintf("host.docker.internal:%d", port())
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, dialMethodUnavailableError
	}
	return conn, err
}

func port() int {
	return mustGetenvInt("UPLIFT_PORT")
}
