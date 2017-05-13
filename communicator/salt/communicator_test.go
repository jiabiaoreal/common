// +build !race

package salt

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	log "github.com/golang/glog"

	"we.com/jiabiao/common/communicator/remote"
	"we.com/jiabiao/common/communicator/types"
)

func TestNew_Invalid(t *testing.T) {
	r := types.ConnInfo{
		"type":         "salt",
		"host":         "10.10.10.145",
		"saltFileRoot": "/srv/salt",
		"timeout":      "30s",
	}

	c, err := New(r)
	if err != nil {
		t.Fatalf("error creating communicator: %s", err)
	}

	err = c.Connect(nil)
	if err == nil {
		t.Fatal("should have had an error connecting")
	}
}

func TestStart(t *testing.T) {
	r := types.ConnInfo{
		"type":         "salt",
		"host":         "10.10.10.146",
		"saltFileRoot": "/srv/salt",
		"timeout":      "30s",
	}

	c, err := New(r)
	if err != nil {
		t.Fatalf("error creating communicator: %s", err)
	}

	var cmd remote.Cmd
	stdout := new(bytes.Buffer)
	cmd.Command = "echo foo"
	cmd.Stdout = stdout

	err = c.Start(&cmd)
	if err != nil {
		t.Fatalf("error executing remote command: %s", err)
	}
}

func TestScriptPath(t *testing.T) {
	cases := []struct {
		Input   string
		Pattern string
	}{
		{
			"/tmp/script.sh",
			`^/tmp/script\.sh$`,
		},
		{
			"/tmp/script_%RAND%.sh",
			`^/tmp/script_(\d+)\.sh$`,
		},
	}

	for _, tc := range cases {
		r := types.ConnInfo{
			"type":         "salt",
			"host":         "10.10.10.146",
			"saltFileRoot": "/srv/salt",
			"timeout":      "30s",
			"scriptPath":   tc.Input,
		}
		comm, err := New(r)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		output := comm.ScriptPath()

		match, err := regexp.Match(tc.Pattern, []byte(output))
		if err != nil {
			t.Fatalf("bad: %s\n\nerr: %s", tc.Input, err)
		}
		if !match {
			t.Fatalf("bad: %s\n%s\n%s", tc.Input, output, comm.connInfo.ScriptPath)
		}
	}
}

func TestScriptPath_randSeed(t *testing.T) {
	// Pre GH-4186 fix, this value was the deterministic start the pseudorandom
	// chain of unseeded math/rand values for Int31().
	staticSeedPath := "/tmp/terraform_1298498081.sh"
	r := types.ConnInfo{
		"type":         "salt",
		"host":         "10.10.10.146",
		"saltFileRoot": "/srv/salt",
		"timeout":      "30s",
	}
	c, err := New(r)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	path := c.ScriptPath()
	if path == staticSeedPath {
		t.Fatalf("rand not seeded! got: %s", path)
	}
}

func TestFileUpload(t *testing.T) {
	// Pre GH-4186 fix, this value was the deterministic start the pseudorandom
	// chain of unseeded math/rand values for Int31().
	r := types.ConnInfo{
		"type":         "salt",
		"host":         "10.10.10.146",
		"saltFileRoot": "/srv/salt",
		"timeout":      "30s",
	}
	c, err := New(r)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	tmpfile, err := ioutil.TempFile("/tmp/", "terraform-test")
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	buf := bytes.NewBufferString("R29waGVycyBydWxlIQ==")
	c.Upload(tmpfile.Name(), buf)

	var cmd remote.Cmd
	stdout := new(bytes.Buffer)
	cmd.Command = fmt.Sprintf("ls %s", tmpfile.Name())
	cmd.Stdout = stdout

	err = c.Start(&cmd)
	if err != nil {
		t.Fatalf("error executing remote command: %s", err)
	}
}

func TestDirUpload(t *testing.T) {
	// Pre GH-4186 fix, this value was the deterministic start the pseudorandom
	// chain of unseeded math/rand values for Int31().
	r := types.ConnInfo{
		"type":         "salt",
		"host":         "10.10.10.146",
		"saltFileRoot": "/srv/salt",
		"timeout":      "30s",
	}
	c, err := New(r)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	tmpfile, err := ioutil.TempDir("/tmp/", "terraform-dir")

	err = c.UploadDir(tmpfile, "/opt/go/src/we.com/jiabiao/common/communicator")

	if err != nil {
		t.Fatalf("err upload dir: %v", err)
	}

	var cmd remote.Cmd
	stdout := new(bytes.Buffer)
	cmd.Command = fmt.Sprintf("ls %s", tmpfile)
	cmd.Stdout = stdout

	log.Info(stdout.String())

	err = c.Start(&cmd)
	if err != nil {
		t.Fatalf("error executing remote command: %s", err)
	}
}
