package salt

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"we.com/jiabiao/common/communicator/remote"
	"we.com/jiabiao/common/communicator/types"
	log "github.com/golang/glog"
)

// host last use time of host ip

var (
	connectionPool     = map[string]time.Time{}
	connectionKeepTime = 15 * time.Minute
)

const (
	// DefaultShebang is added at the top of a SSH script file
	DefaultShebang = "#!/bin/sh\n"
)

// Communicator represents the SSH communicator
type Communicator struct {
	connInfo *connectionInfo
	rand     *rand.Rand
}

// New creates a new communicator implementation with salt
// this only checks if the target host is a salt minion
func New(s types.ConnInfo) (*Communicator, error) {
	ci, err := parseConnectionInfo(s)

	if err != nil {
		return nil, err
	}

	comm := &Communicator{
		connInfo: ci,
		// Seed our own rand source so that script paths are not deterministic
		rand: rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	return comm, nil
}

func salt_ping(host string, timeout time.Duration) error {
	var err error
	if host == "" {
		err = fmt.Errorf("host is empty, please specify a host to exec on")
		return err
	}

	args := []string{"-L", host, "test.ping"}

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	_, err = execSaltCmd(args, timeout)
	return err
}

func (c *Communicator) salt_start(rcmd *remote.Cmd) (err error) {
	if c == nil || c.connInfo.Host == "" {
		err = fmt.Errorf("host is empty, please specify a host to exec on")
		return
	}

	if rcmd == nil || rcmd.Command == "" {
		err = fmt.Errorf("cmd is nil or Command is empty")
		return
	}

	cmdpath := "salt"
	args := []string{"-L", c.connInfo.Host, "--out=text", "cmd.run", rcmd.Command}

	cmd := exec.Command(cmdpath, args...)
	cmd.Stdout = rcmd.Stdout
	cmd.Stdin = rcmd.Stdin
	cmd.Stderr = rcmd.Stderr

	err = cmd.Start()
	if err != nil {
		return err
	}

	if c.connInfo.Timeout <= 0 {
		c.connInfo.Timeout = DefaultTimeout
	}

	go func() {
		err := waitCmdWithTimeout(cmd, c.connInfo.Timeout)
		rc := 0
		if err != nil {
			log.Warningf("process done with error = %v", err)
			if exiterr, ok := err.(*exec.ExitError); ok {
				// The program has exited with an exit code != 0
				// There is no plattform independent way to retrieve
				// the exit code, but the following will work on Unix
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					rc = status.ExitStatus()
				}
			}
			if rc == 0 {
				rc = 255
			}
		} else {
			log.V(10).Info("process done gracefully without error")
		}
		rcmd.SetExited(rc)
	}()

	connectionPool[c.connInfo.Host] = time.Now()
	return nil
}

// Connect implementation of communicator.Communicator interface
func (c *Communicator) Connect(o types.UIOutput) (err error) {
	if c == nil {
		return fmt.Errorf("communicator is nil, New one first")
	}

	if v, ok := connectionPool[c.connInfo.Host]; ok {
		if time.Now().Before(v.Add(connectionKeepTime)) {
			log.V(10).Infof("salt client buffer,  just return for connect")
			return nil
		}
	}

	err = salt_ping(c.connInfo.Host, c.connInfo.Timeout)
	if err == nil {
		connectionPool[c.connInfo.Host] = time.Now()
	}
	return err
}

// Disconnect implementation of communicator.Communicator interface
// TODO: if it has unfinished job, kill that first
func (c *Communicator) Disconnect() error {
	return nil
}

// Timeout implementation of communicator.Communicator interface
func (c *Communicator) Timeout() time.Duration {
	return c.connInfo.Timeout
}

// ScriptPath implementation of communicator.Communicator interface
func (c *Communicator) ScriptPath() string {
	return strings.Replace(
		c.connInfo.ScriptPath, "%RAND%",
		strconv.FormatInt(int64(c.rand.Int31()), 10), -1)
}

// Start implementation of communicator.Communicator interface
func (c *Communicator) Start(cmd *remote.Cmd) error {
	log.Infof("starting remote command: %s", cmd.Command)
	return c.salt_start(cmd)
}

// Upload implementation of communicator.Communicator interface
func (c *Communicator) Upload(path string, input io.Reader) error {
	return c.saltUploadFile(path, input, c.connInfo.SaltFileRoot)
}

// UploadScript implementation of communicator.Communicator interface
func (c *Communicator) UploadScript(path string, input io.Reader) error {
	reader := bufio.NewReader(input)
	prefix, err := reader.Peek(2)
	if err != nil {
		return fmt.Errorf("Error reading script: %s", err)
	}

	var script bytes.Buffer
	if string(prefix) != "#!" {
		script.WriteString(DefaultShebang)
	}

	script.ReadFrom(reader)
	if err := c.Upload(path, &script); err != nil {
		return err
	}

	var stdout, stderr bytes.Buffer
	cmd := &remote.Cmd{
		Command: fmt.Sprintf("chmod 0777 %s", path),
		Stdout:  &stdout,
		Stderr:  &stderr,
	}
	if err := c.Start(cmd); err != nil {
		return fmt.Errorf(
			"Error chmodding script file to 0777 in remote "+
				"machine: %s", err)
	}
	cmd.Wait()
	if cmd.ExitStatus != 0 {
		return fmt.Errorf(
			"Error chmodding script file to 0777 in remote "+
				"machine %d: %s %s", cmd.ExitStatus, stdout.String(), stderr.String())
	}

	return nil
}

// UploadDir implementation of communicator.Communicator interface
func (c *Communicator) UploadDir(dst string, src string) error {
	log.V(10).Infof("Uploading dir '%s' to '%s'", src, dst)
	tf, err := ioutil.TempFile(c.connInfo.SaltFileRoot, "terraform-upload")
	if err != nil {
		return fmt.Errorf("Error creating temporary file for upload: %s", err)
	}
	tmpfile := tf.Name()
	defer os.Remove(tmpfile)
	tf.Close()

	log.V(10).Info("tar czf  the source dir")
	args := []string{"czf", tmpfile, "."}
	cmdpath := "tar"
	cmd := exec.Command(cmdpath, args...)
	cmd.Dir = src

	if err := cmd.Start(); err != nil {
		return err
	}

	if err = waitCmdWithTimeout(cmd, DefaultTimeout); err != nil {
		return err
	}

	log.V(10).Info("upload tar file to target")
	srcbase := filepath.Base(tmpfile)
	tmptf := filepath.Join("/tmp", srcbase)
	if err = c.uploadFile(tmptf, srcbase); err != nil {
		return err
	}

	log.V(10).Info("extract tar file and rm tmp file")
	tag := "CMDSUCCESS"
	cmdstr := fmt.Sprintf(`mkdir -p "%s" && tar xzf %s -C "%s" && /bin/rm -f "%s" && echo %s`, dst, tmptf, dst, tmptf, tag)
	args = []string{"--out=txt", "-L", c.connInfo.Host, "cmd.run", cmdstr}
	out, err := execSaltCmd(args, c.connInfo.Timeout)

	if err != nil {
		return err
	}

	if !strings.Contains(out, tag) {
		err = fmt.Errorf("extract file err: %s", out)
	}
	log.V(10).Infof("upload dir: %s", out)

	return err
}

func waitCmdWithTimeout(cmd *exec.Cmd, timeout time.Duration) (err error) {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	select {
	case <-time.After(timeout):
		if err = cmd.Process.Kill(); err != nil {
			log.Fatal("failed to kill: ", err)
		}
		log.Warning("process killed as timeout reached")
		err = fmt.Errorf("process killed as timeout reached")
	case err = <-done:
		if err != nil {
			log.Warningf("process: %s(%v) done with error = %v", cmd.Path, cmd.Args, err)
		} else {
			log.V(10).Info("process done gracefully without error")
		}
	}
	return
}

func execCmd(cmdpath string, args []string, timeout time.Duration) (out string, err error) {
	if cmdpath == "" {
		return "", fmt.Errorf("cmd cannot be empty")
	}

	cmd := exec.Command(cmdpath, args...)
	var outbuf bytes.Buffer
	cmd.Stdout = &outbuf

	log.V(10).Infof("start to exec cmd: %s %v", cmdpath, args)
	err = cmd.Start()
	// here we actually do not need to check the error
	if err != nil {
		log.Warning(err)
		return
	}
	err = waitCmdWithTimeout(cmd, timeout)
	log.V(10).Infof("end exec cmd: %s %v", cmdpath, args)
	out = outbuf.String()
	return
}

func execSaltCmd(args []string, timeout time.Duration) (out string, err error) {
	cmdpath := "salt"
	return execCmd(cmdpath, args, timeout)
}

func (c *Communicator) uploadFile(dst string, src string) error {
	log.V(10).Infof("start  file %s to upload to remote %s", src, dst)
	abssrc := filepath.Join(c.connInfo.SaltFileRoot, src)

	if _, err := os.Stat(abssrc); os.IsNotExist(err) {
		return fmt.Errorf("src file %s not exist", src)
	}

	src = "salt://" + src
	args := []string{"--out=txt", "-L", c.connInfo.Host, "cp.get_file", src, dst, "gzip=5", "makedirs=True"}

	timeout := c.connInfo.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	out, err := execSaltCmd(args, timeout)
	log.V(10).Info(out)
	if err != nil {
		err = fmt.Errorf("upload file err: %v, %s", err, out)
	}
	return err
}

func (c *Communicator) saltUploadFile(dst string, src io.Reader, saltFileRoot string) error {
	// Create a temporary file where we can copy the contents of the src
	// so that we can determine the length, since SCP is length-prefixed.
	tf, err := ioutil.TempFile(saltFileRoot, "terraform-upload")
	if err != nil {
		return fmt.Errorf("Error creating temporary file for upload: %s", err)
	}
	defer os.Remove(tf.Name())
	defer tf.Close()

	log.V(10).Info("Copying input data into temporary salt file root path so we copy with cp.file")
	if _, err := io.Copy(tf, src); err != nil {
		return err
	}

	// Sync the file so that the contents are definitely on disk, then
	// slat cp.get_file
	if err := tf.Sync(); err != nil {
		return fmt.Errorf("Error creating temporary file for upload: %s", err)
	}

	return c.uploadFile(dst, filepath.Base(tf.Name()))
}
