package repo

import (
	"os/exec"
	"testing"
	"time"
)

func Test_execCmd(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	out, err := execCmd(cmd, 1*time.Second, 5*time.Second)

	t.Log(out)
	if err == nil {
		t.Fatalf("err should not be nil")
	}

	cmd = exec.Command("sleep", "10")
	out, err = execCmd(cmd, 5*time.Second, 1*time.Second)

	if out != "waitTimeout" {
		t.Fatalf("out should WaitTimeout, get %s", out)
	}

	if err != nil {
		t.Fatalf("expect err nil, get %v", err)
	}

	cmd = exec.Command("sleep", "1")
	out, err = execCmd(cmd, 5*time.Second, 5*time.Second)

	t.Log(out)
	if err != nil {
		t.Fatalf("expect err nil, get %v", err)
	}
}
