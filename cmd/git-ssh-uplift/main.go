package main

import (
	"context"
	"encoding/gob"
	"errors"
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
			c.Close()
			<-sem
			if err != nil {
				log.Printf("proxy: %s", err)
			}
		}()
	}
	return nil
}

var services = map[proto.GitService]string{
	proto.GitUploadPack:  "git-upload-pack",
	proto.GitReceivePack: "git-receive-pack",
}

func proxy(ctx context.Context, rw io.ReadWriter) error {
	hdr := proto.Header{}
	dec := gob.NewDecoder(rw)
	if err := dec.Decode(&hdr); err != nil {
		return fmt.Errorf("bad header: %s", err)
	}
	svcName, ok := services[hdr.GitService]
	if !ok {
		return fmt.Errorf("bad header: service: %d", hdr.GitService)
	}
	if hdr.Hostname == "" {
		return errors.New("bad header: missing hostname")
	}
	uh := hdr.Hostname
	if hdr.Username != "" {
		uh = hdr.Username + "@" + hdr.Hostname
	}
	repo := hdr.RepositoryPath
	if repo == "" {
		repo = "."
	}

	args := []string{"ssh", "-x", uh, fmt.Sprintf(`%s '%s'`, svcName, repo)}
	return execProcCopyStdio(procSpec{args: args}, rw)
}

type procSpec struct {
	args []string // first element is program name
	attr *os.ProcAttr
}

func execProcCopyStdio(ps procSpec, rw io.ReadWriter) error {

	// Cmd.Wait() would ordinarily wait for stdin to hit EOF.  In our case, the
	// subprocess stdin is attached to a network socket that remains open after
	// the subprocess terminates.  Cmd.Wait() deadlocks.

	qname, err := exec.LookPath(ps.args[0])
	if err != nil {
		return err
	}

	ir, iw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer ir.Close()
	defer iw.Close()

	or, ow, err := os.Pipe()
	if err != nil {
		return err
	}
	defer or.Close()
	defer ow.Close()

	if ps.attr == nil {
		ps.attr = &os.ProcAttr{}
	}
	ps.attr.Files = []*os.File{ir, ow, os.Stderr}

	proc, err := os.StartProcess(qname, ps.args, ps.attr)
	if err != nil {
		return err
	}

	eg := &errgroup.Group{}
	eg.Go(func() error {
		state, err := proc.Wait()
		if err != nil {
			return err
		}
		if !state.Success() {
			return fmt.Errorf("%s exited with code %d", ps.args[0], state.ExitCode())
		}
		return nil
	})
	eg.Go(func() error { _, err := io.Copy(rw, or); return err })
	eg.Go(func() error { _, err := io.Copy(iw, rw); return err })
	return eg.Wait()
}

func isClosed(err error) bool {
	// go bug 4373
	opError, ok := err.(*net.OpError)
	if !ok {
		return false
	}
	return strings.Contains(opError.Error(), "closed")
}
