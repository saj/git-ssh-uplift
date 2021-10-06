package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"golang.org/x/sync/errgroup"

	"github.com/saj/git-ssh-uplift/internal/proto"
)

type procArgs []string

func sshProcArgs(hdr proto.Header) (procArgs, error) {
	if hdr.Hostname == "" {
		return nil, errors.New("bad header: missing hostname")
	}
	svcname, ok := services[hdr.GitService]
	if !ok {
		return nil, fmt.Errorf("bad header: service: %d", hdr.GitService)
	}
	userhost := hdr.Hostname
	if hdr.Username != "" {
		userhost = hdr.Username + "@" + hdr.Hostname
	}
	repo := "."
	if hdr.RepositoryPath != "" {
		repo = hdr.RepositoryPath
	}
	return procArgs{
		"ssh", "-x", userhost,
		svcname + " '" + repo + "'",
	}, nil
}

var services = map[proto.GitService]string{
	proto.GitUploadPack:  "git-upload-pack",
	proto.GitReceivePack: "git-receive-pack",
}

func (a procArgs) ExecPiped(ctx context.Context, rw io.ReadWriteCloser) error {
	cmd := exec.CommandContext(ctx, a[0], a[1:]...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	eg := errgroup.Group{}
	eg.Go(func() error { _, err := io.Copy(stdin, rw); return err })
	eg.Go(func() error {
		defer rw.Close() // unblock stdin copy
		_, err := io.Copy(rw, stdout)
		return err
	})
	eg.Go(func() error { return cmd.Wait() })
	return eg.Wait()
}
