package main

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/saj/git-ssh-uplift/internal/proto"
)

func newLimiter(maxconns uint) limiter {
	if maxconns < 1 {
		return dummyLimiter{}
	}
	return make(finiteLimiter, maxconns)
}

type limiter interface {
	Acquire()
	Done()
}

type dummyLimiter struct{}

func (l dummyLimiter) Acquire() {}
func (l dummyLimiter) Done()    {}

type finiteLimiter chan struct{}

func (l finiteLimiter) Acquire() { l <- struct{}{} }
func (l finiteLimiter) Done()    { <-l }

func serve(ctx context.Context, lis *net.TCPListener, lim limiter) error {
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			lis.Close()
		}
	}()

loop:
	for {
		lim.Acquire()
		conn, err := lis.Accept()
		if errors.Is(err, net.ErrClosed) {
			break loop
		}
		if err != nil {
			return err
		}
		go func(conn net.Conn) {
			defer lim.Done()
			if err := proxy(ctx, conn); err != nil {
				log.Printf("proxy: %s", err)
			}
		}(conn)
	}
	return nil
}

func proxy(ctx context.Context, rw io.ReadWriteCloser) error {
	hdr := proto.Header{}
	if err := gob.NewDecoder(rw).Decode(&hdr); err != nil {
		return fmt.Errorf("bad header: %s", err)
	}
	args, err := sshProcArgs(hdr)
	if err != nil {
		return err
	}
	return args.ExecPiped(ctx, rw)
}
