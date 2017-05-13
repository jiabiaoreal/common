package salt

import (
	"fmt"
	"log"
	"time"

	"we.com/jiabiao/common/communicator/types"

	"github.com/mitchellh/mapstructure"
)

const (
	// DefaultUser is used if there is no user given
	DefaultUser = "root"

	// DefaultScriptPath is used as the path to copy the file to
	// for remote execution if not provided otherwise.
	DefaultScriptPath = "/tmp/terraform_%RAND%.sh"

	// DefaultTimeout is used if there is no timeout given
	DefaultTimeout = 5 * time.Minute
)

// connectionInfo is decoded from the ConnInfo of the resource. These are the
// only keys we look at. If a KeyFile is given, that is used instead
// of a password.
type connectionInfo struct {
	User         string `mapstructure:"user"`
	Host         string
	ScriptPath   string        `mapstructure:"scriptPath"`
	SaltFileRoot string        `mapstructure:"saltFileRoot"`
	TimeoutStr   string        `mapstructure:"timeout"`
	Timeout      time.Duration `mapstructure:"-"`
}

// parseConnectionInfo is used to convert the ConnInfo of the InstanceState into
// a ConnectionInfo struct
func parseConnectionInfo(ci types.ConnInfo) (*connectionInfo, error) {
	connInfo := &connectionInfo{}
	decConf := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           connInfo,
	}
	dec, err := mapstructure.NewDecoder(decConf)
	if err != nil {
		return nil, err
	}
	if err := dec.Decode(ci); err != nil {
		return nil, err
	}

	if connInfo.TimeoutStr != "" {
		connInfo.Timeout = safeDuration(connInfo.TimeoutStr, DefaultTimeout)
	} else {
		connInfo.Timeout = DefaultTimeout
	}

	if connInfo.Host == "" {
		return nil, fmt.Errorf("host is empty")
	}

	if connInfo.SaltFileRoot == "" {
		return nil, fmt.Errorf("salt file root not set")
	}

	if connInfo.User == "" {
		connInfo.User = DefaultUser
	}
	if connInfo.ScriptPath == "" {
		connInfo.ScriptPath = DefaultScriptPath
	}

	return connInfo, nil
}

// safeDuration returns either the parsed duration or a default value
func safeDuration(dur string, defaultDur time.Duration) time.Duration {
	d, err := time.ParseDuration(dur)
	if err != nil {
		log.Printf("Invalid duration '%s', using default of %s", dur, defaultDur)
		return defaultDur
	}
	return d
}
