package proto

type GitService uint8

const (
	GitUploadPack  GitService = iota // client <- pull <- server
	GitReceivePack                   // client -> push -> server
)

type Header struct {
	GitService
	Username       string
	Hostname       string
	RepositoryPath string
}
