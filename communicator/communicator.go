package communicator

import (
	"fmt"
	"io"
	"time"

	"we.com/jiabiao/common/communicator/remote"
	"we.com/jiabiao/common/communicator/salt"
	"we.com/jiabiao/common/communicator/ssh"
	"we.com/jiabiao/common/communicator/types"
)

// Communicator is an interface that must be implemented by all communicators
// used for any of the provisioners
type Communicator interface {
	// Connect is used to setup the connection
	Connect(types.UIOutput) error

	// Disconnect is used to terminate the connection
	Disconnect() error

	// Timeout returns the configured connection timeout
	Timeout() time.Duration

	// ScriptPath returns the configured script path
	ScriptPath() string

	// Start executes a remote command in a new session
	Start(*remote.Cmd) error

	// Upload is used to upload a single file
	Upload(string, io.Reader) error

	// UploadScript is used to upload a file as a executable script
	UploadScript(string, io.Reader) error

	// UploadDir is used to upload a directory
	UploadDir(string, string) error
}

// New returns a configured Communicator or an error if the connection type is not supported
func New(ci types.ConnInfo) (Communicator, error) {
	connType := ci["type"]
	switch connType {
	case "ssh", "": // The default connection type is ssh, so if connType is empty use ssh
		return ssh.New(ci)
	case "salt":
		return salt.New(ci)
	default:
		return nil, fmt.Errorf("connection type '%s' not supported", connType)
	}
}
