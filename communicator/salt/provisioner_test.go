package salt

import (
	"testing"
	"time"

	"we.com/jiabiao/common/communicator/types"
)

func TestProvisioner_connInfo(t *testing.T) {
	r := types.ConnInfo{
		"type":         "salt",
		"user":         "root",
		"saltFileRoot": "/tmp",
		"host":         "127.0.0.1",
		"timeout":      "30s",
	}

	conf, err := parseConnectionInfo(r)
	if err != nil {
		t.Fatalf("bad: %v", err)
	}
	if conf.User != "root" {
		t.Fatalf("bad: %v", conf)
	}
	if conf.Host != "127.0.0.1" {
		t.Fatalf("bad: %v", conf)
	}
	if conf.Timeout != 30*time.Second {
		t.Fatalf("bad: %v", conf)
	}
	if conf.ScriptPath != DefaultScriptPath {
		t.Fatalf("bad: %v", conf)
	}
}
