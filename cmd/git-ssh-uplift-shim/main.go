package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/saj/git-ssh-uplift/internal/proto"
)

func usage() {
	prog := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "usage: %s <git-service> <user|-> <repo-host> <repo-path>\n", prog)
	os.Exit(2)
}

func init() {
	log.SetFlags(0)
	log.SetPrefix("git-ssh-uplift-shim: ")
}

func main() {
	args := os.Args[1:]
	if len(args) != 4 {
		usage()
	}
	var (
		svcname = args[0]
		user    = args[1]
		host    = args[2]
		repo    = args[3]
	)

	svc, ok := services[svcname]
	if !ok {
		log.Fatalf("invalid git service: %s", svcname)
	}

	if user == "-" {
		user = ""
	}

	conn, err := dial()
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	hdr := proto.Header{
		GitService:     svc,
		Username:       user,
		Hostname:       host,
		RepositoryPath: repo,
	}
	if err := proxy(hdr, conn); err != nil {
		log.Fatalf("proxy: %s", err)
	}
}

var services = map[string]proto.GitService{
	"git-upload-pack":  proto.GitUploadPack,
	"git-receive-pack": proto.GitReceivePack,
}

func proxy(hdr proto.Header, rw io.ReadWriter) error {
	if err := gob.NewEncoder(rw).Encode(hdr); err != nil {
		return err
	}
	return copyStdio(rw)
}

func copyStdio(rw io.ReadWriter) error {
	eg := errgroup.Group{}
	eg.Go(func() error {
		_, err := io.Copy(rw, os.Stdin)
		return err
	})
	eg.Go(func() error {
		_, err := io.Copy(os.Stdout, rw)
		return err
	})
	return eg.Wait()
}
