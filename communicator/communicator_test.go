package communicator

import (
	"testing"

	"we.com/jiabiao/common/communicator/types"
)

func TestCommunicator_new(t *testing.T) {
	r := types.ConnInfo{
		"type": "telnet",
	}
	if _, err := New(r); err == nil {
		t.Fatalf("expected error with telnet")
	}

	r["type"] = "salt"
	if _, err := New(r); err == nil {
		t.Fatalf("err: %v", err)
	}

	r["type"] = "salt"
	r["host"] = "abcd"
	r["saltFileRoot"] = "/tmp"
	if _, err := New(r); err != nil {
		t.Fatalf("err: %v", err)
	}

	r["type"] = "ssh"
	if _, err := New(r); err != nil {
		t.Fatalf("err: %v", err)
	}
}
