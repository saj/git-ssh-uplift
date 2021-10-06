package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/saj/git-ssh-uplift/internal/proto"
)

var (
	cmdargs  = kingpin.Arg("cmdargs", "Execute a command name (with arguments) after the uplift proxy begins serving, then self-terminate when the command completes.  UPLIFT_PORT is automatically set in the process environment in order to facilitate connections from the uplift shim.").Strings()
	bindAddr = kingpin.Flag("bind", "Bind the uplift proxy to a specific local address and/or TCP port.  host defaults to all addresses.  port is randomly chosen if omitted.").PlaceHolder("[host]:[port]").TCP()
	connsMax = kingpin.Flag("conns-max", "Set the maximum number of concurrent connections to the proxy.  Any new connections that would violate this limit are not accepted.").Default("10").Uint()
)

func init() {
	log.SetFlags(0)
	log.SetPrefix("git-ssh-uplift: ")
}

func main() {
	kingpin.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, unix.SIGINT, unix.SIGTERM)
		<-sigs
		cancel()
	}()

	l, err := net.ListenTCP("tcp", *bindAddr)
	if err != nil {
		log.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port

	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() error { return serve(ectx, l) })
	if len(*cmdargs) > 0 {
		eg.Go(func() error {
			err := fexec(ectx, port, (*cmdargs)[0], (*cmdargs)[1:]...)
			cancel()
			return err
		})
	} else {
		log.Printf("listening on port %d - ^C to exit", port)
	}
	if err := eg.Wait(); err != nil {
		log.Fatal(err)
	}
}

func fexec(ctx context.Context, port int, name string, arg ...string) error {
	env := os.Environ()
	env = append(env, fmt.Sprintf("SSH_UPLIFT_PORT=%d", port))
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func serve(ctx context.Context, l *net.TCPListener) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-ctx.Done():
		}
		l.Close()
	}()
	sem := make(chan struct{}, int(*connsMax))
loop:
	for {
		sem <- struct{}{}
		c, err := l.Accept()
		if isClosed(err) {
			break loop
		}
		if err != nil {
			return err
		}
		go func() {
			err := proxy(ctx, c)
			<-sem
			if err != nil {
				log.Printf("proxy: %s", err)
			}
		}()
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

func isClosed(err error) bool {
	// go bug 4373
	opError, ok := err.(*net.OpError)
	if !ok {
		return false
	}
	return strings.Contains(opError.Error(), "closed")
}
