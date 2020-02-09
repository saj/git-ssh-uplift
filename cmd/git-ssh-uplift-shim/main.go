package main

import (
	"bufio"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/saj/git-ssh-uplift/internal/proto"
)

const dialTimeout = 3 * time.Second

func usage() {
	prog := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "usage: %s <git-service> <user|-> <repo-host> <repo-path>\n", prog)
	os.Exit(2)
}

func init() {
	log.SetFlags(0)
	log.SetPrefix("git-ssh-uplift-shim: ")
}

var services = map[string]proto.GitService{
	"git-upload-pack":  proto.GitUploadPack,
	"git-receive-pack": proto.GitReceivePack,
}

func main() {
	args := os.Args[1:]
	if len(args) != 4 {
		usage()
	}
	var (
		svcName = args[0]
		user    = args[1]
		host    = args[2]
		repo    = args[3]
	)
	svc, ok := services[svcName]
	if !ok {
		log.Fatalf("invalid git service: %s", svcName)
	}
	if user == "-" {
		user = ""
	}
	hdr := proto.Header{
		GitService:     svc,
		Username:       user,
		Hostname:       host,
		RepositoryPath: repo,
	}
	c, err := dialAutodetectProxyAddress()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	if err := proxy(hdr, c); err != nil {
		log.Fatalf("proxy: %s", err)
	}
}

func proxy(hdr proto.Header, rw io.ReadWriter) error {
	enc := gob.NewEncoder(rw)
	if err := enc.Encode(hdr); err != nil {
		return err
	}
	return copyStdio(rw)
}

func copyStdio(rw io.ReadWriter) error {
	done := make(chan error, 2)
	go func() {
		_, err := io.Copy(rw, os.Stdin)
		done <- err
	}()
	go func() {
		_, err := io.Copy(os.Stdout, rw)
		done <- err
	}()
	return <-done
}

func dialAutodetectProxyAddress() (net.Conn, error) {
	port, err := getenvInt("UPLIFT_PORT")
	if err != nil {
		return nil, err
	}

	if host := os.Getenv("UPLIFT_HOST"); host != "" {
		return net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), dialTimeout)
	}

	if runtime.GOOS == "linux" {
		// see https://github.com/moby/moby/pull/40007
		addrs := make([]string, 0, 2)
		if gw := defaultGatewayLinux(); gw != "" {
			addrs = append(addrs, fmt.Sprintf("%s:%d", gw, port))
		}
		addrs = append(addrs, fmt.Sprintf("host.docker.internal:%d", port))
		for _, addr := range addrs {
			c, err := net.DialTimeout("tcp", addr, dialTimeout)
			if err != nil {
				continue
			}
			return c, nil
		}
	}

	return nil, errors.New("unable to autodetect proxy address - try again with UPLIFT_HOST")
}

func defaultGatewayLinux() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() { // skip header
		return ""
	}
	for s.Scan() {
		f := strings.Split(s.Text(), "\t")
		if len(f) < 3 {
			continue
		}
		var (
			destination = f[1]
			gateway     = f[2]
		)
		if destination != "00000000" {
			continue
		}
		d, err := strconv.ParseUint(gateway, 16, 32)
		if err != nil {
			continue
		}
		ip := make(net.IP, 4)
		binary.LittleEndian.PutUint32(ip, uint32(d))
		return ip.String()
	}
	return ""
}

func getenvInt(name string) (int, error) {
	s := os.Getenv(name)
	if s == "" {
		return 0, fmt.Errorf("%s missing from environment", name)
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %s", name, err)
	}
	return i, nil
}
