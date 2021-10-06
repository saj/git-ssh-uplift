package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"golang.org/x/sync/errgroup"
)

const desc = `Launch the git-ssh-uplift proxy.

SYNOPSIS

  git-ssh-uplift [flags] [prog [arg ...]]

DESCRIPTION

For rationale, see https://github.com/saj/git-ssh-uplift

The uplift proxy may be invoked with or without positional arguments.
Positional arguments denote a command name (and its arguments) to be forked as a
child of the uplift proxy.  UPLIFT_PORT is set in the environment of the child.
The uplift proxy automatically shuts down after its child terminates.  Without
positional arguments, the uplift proxy will continue serving until it is
explicitly interrupted (with SIGINT or SIGTERM).
`

type Cmd struct {
	Args     []string    `kong:"arg,optional"`
	Bind     BindAddress `kong:"default='0.0.0.0:',placeholder='[host]:[port]',help='Bind the uplift proxy to a particular local address and/or TCP port.  host defaults to all addresses.  port is randomly chosen if omitted.'"`
	ConnsMax uint        `kong:"default='10',help='Limit the maximum number of concurrent connections to the proxy (if value is positive).'"`
}

type BindAddress net.TCPAddr

func (f *BindAddress) UnmarshalText(text []byte) error {
	addr, err := net.ResolveTCPAddr("tcp", string(text))
	if err != nil {
		return err
	}
	*f = *(*BindAddress)(addr)
	return nil
}

type CmdContext struct {
	Canceler context.Context
}

func (cmd *Cmd) Run(cmdcon CmdContext) error {
	ctx, cancel := context.WithCancel(cmdcon.Canceler)
	defer cancel()

	lis, err := net.ListenTCP("tcp", (*net.TCPAddr)(&cmd.Bind))
	if err != nil {
		return err
	}
	port := lis.Addr().(*net.TCPAddr).Port

	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() error { return serve(ectx, lis, newLimiter(cmd.ConnsMax)) })
	if len(cmd.Args) > 0 {
		eg.Go(func() error {
			args := procArgs(cmd.Args)
			err := args.ExecUplifted(ectx, port)
			cancel()
			return err
		})
	} else {
		log.Printf("listening on port %d - ^C to exit", port)
	}
	return eg.Wait()
}

func init() {
	log.SetFlags(0)
	log.SetPrefix("git-ssh-uplift: ")
}

func main() {
	os.Exit(run())
}

func run() int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-signals
		cancel()
	}()

	var cli Cmd
	kong.Parse(&cli, kong.Description(desc))
	cmdcon := CmdContext{Canceler: ctx}
	if err := cli.Run(cmdcon); err != nil {
		log.Print(err)
		return 1
	}
	return 0
}
