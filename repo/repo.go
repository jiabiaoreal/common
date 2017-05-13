package repo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sync"
	"time"

	log "github.com/golang/glog"
)

var (
	repolock             sync.Mutex
	GitPrexHistoryCommit = regexp.MustCompile("^0{1,}$")
)

const (
	PathNotExists = iota
	PathFormatError
	NotARepo
	BranchNotExists
	CloneError
	CheckOutError
	PermissionError
	RemoveDirError
	FetchError
	WaitTimeout
	UnknownError
)

type RepoError struct {
	Code int
	err  error
	Msg  string
}

func (re *RepoError) ErrorOrNil() error {
	if re != nil {
		return fmt.Errorf("Err code: %d, %s, %s", re.Code, re.Msg, re.err.Error())
	}

	return nil
}

func (re *RepoError) Error() string {
	if re != nil {
		return fmt.Sprintf("Err code: %d, %s, %s", re.Code, re.Msg, re.err.Error())
	} else {
		return "RepoError is nil"
	}
}

//InitRepo clone a remote repo to localpath
// release repolock before call this function
// command execute at most timeout, and this function will return before wait
func InitRepo(repourl, localPath string, timeout time.Duration, wait time.Duration) *RepoError {
	dir := path.Dir(localPath)
	repolock.Lock()
	defer repolock.Unlock()

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return &RepoError{
			Code: PermissionError,
			err:  err,
			Msg:  "don't have permission to create dir",
		}
	}

	cmd := exec.Command("git", "clone", repourl, localPath)
	log.Infof("start init repo:%s, %s", localPath, repourl)
	output, err := execCmd(cmd, timeout, wait)
	// output, err := cmd.Output()

	if err != nil {
		log.Warningf("error init repo: %v", err)
		return &RepoError{
			Code: CloneError,
			err:  err,
			Msg:  fmt.Sprintf("failed to clone repo %s to %s", repourl, localPath),
		}
	}

	if string(output) == "waitTimeout" {
		log.Warningf("wait timeout init repo: %v", output)
		return &RepoError{
			Code: WaitTimeout,
			err:  err,
			Msg:  fmt.Sprintf("wait timeout for clone repo %s to %s", repourl, localPath),
		}
	}

	log.Info(string(output))
	return nil
}

// UpdateRepo update a local git repo, if not exists, create one
func UpdateRepo(localPath, gitRepo string, timeout time.Duration) *RepoError {
	cpath := path.Clean(localPath)

	_, err := os.Stat(localPath)
	var wait time.Duration
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}

	wait = timeout + 1*time.Minute

	// mkdir if not exists
	if err != nil && os.IsNotExist(err) {
		nerr := InitRepo(gitRepo, cpath, timeout, wait)
		if nerr != nil {
			return nerr
		}
		return nil
	} else if err != nil {
		return &RepoError{
			Code: UnknownError,
			err:  err,
			Msg:  "during stat  dir: " + localPath,
		}
	}

	// dir exists but not a git repo
	cmd := exec.Command("git", "branch")
	cmd.Dir = cpath
	if _, err := cmd.Output(); err != nil {
		log.Warning(err)
		// if dir is empty, rmdir and clone
		if nerr := os.Remove(cpath); nerr != nil {
			return &RepoError{
				Code: RemoveDirError,
				err:  nerr,
				Msg:  "err to remove dir: " + cpath,
			}
		}
		if nerr := InitRepo(gitRepo, cpath, timeout, wait); nerr != nil {
			return nerr
		}
		return nil
	}

	// here we can sure, git repo exists,
	// so just cd, git git  fetch
	repolock.Lock()
	defer repolock.Unlock()
	cmd = exec.Command("git", "fetch", "origin")
	cmd.Dir = cpath
	// execute it
	out, err := execCmd(cmd, timeout, wait)

	log.Info("fetch origin: ", string(out))
	if err != nil {
		return &RepoError{
			Code: FetchError,
			err:  err,
			Msg:  "error occured when fetch origin",
		}
	}
	return nil
}

// if branch exists, return nil
// else return a RepoError
func BranchExists(repo, branch string) *RepoError {
	// check if branch/tag/commit-id exists
	cmd := exec.Command("git", "log", "-1", branch)
	cmd.Dir = repo
	if _, err := cmd.Output(); err != nil {
		return &RepoError{
			Code: BranchNotExists,
			err:  err,
			Msg:  "branch not exists: " + branch,
		}
	}

	return nil
}

// switch2
func Switch2branch(repo, branch string) *RepoError {
	if err := BranchExists(repo, branch); err != nil {
		return err
	}

	return Switch2branch0(repo, branch)
}

// since this will acquire the loc,
// so before call this function, release it
// branch better refer a remote branch
func Switch2branch0(repo, branch string) *RepoError {
	repolock.Lock()
	defer repolock.Unlock()

	// git rev-parse --abbrev-ref HEAD
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repo

	out, err := cmd.Output()

	if err != nil {
		log.Warning(out)
		return &RepoError{
			Code: NotARepo,
			err:  err,
			Msg:  "error when rev-parse --abbrev-ref HEAD",
		}
	}

	// if current branch is not anonymous
	// or we can just reset --hard, without loss someting
	if string(out) != "HEAD" {
		// reset HEAD
		cmd = exec.Command("git", "reset", "--hard", "HEAD")
		cmd.Dir = repo
		_, err = cmd.Output()
		if err != nil {
			return &RepoError{
				Code: UnknownError,
				err:  err,
				Msg:  "when reset --hard HEAD",
			}
		}

		// checkout branch
		cmd = exec.Command("git", "checkout", branch)
		cmd.Dir = repo
		_, err = cmd.Output()

		if err != nil {
			return &RepoError{
				Code: UnknownError,
				err:  err,
				Msg:  "when checkout  " + branch,
			}
		}
	} else {
		cmd = exec.Command("git", "reset", "--hard", branch)
		cmd.Dir = repo
		_, err = cmd.Output()
		if err != nil {
			return &RepoError{
				Code: UnknownError,
				err:  err,
				Msg:  "when reset --hard " + branch,
			}
		}
	}

	return nil
}

func execCmd(cmd *exec.Cmd, timeout time.Duration, wait time.Duration) (string, error) {
	if cmd == nil {
		return "", fmt.Errorf("cmd is nil")
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Start()
	// here we actually do not need to check the error
	if err != nil {
		log.Warning(err)
		return "", err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	var remains time.Duration
	if wait > timeout {
		remains = 0
	} else {
		remains = timeout - wait
	}

	outstr := ""

	select {
	case <-time.After(timeout):
		if err = cmd.Process.Kill(); err != nil {
			log.Fatal("failed to kill: ", err)
		}
		log.Warning("process killed as timeout reached")
		err = fmt.Errorf("process killed as timeout reached")
	case <-time.After(wait):
		log.Warning("wait timeout, process will continue be exceucte timeout")
		outstr = "waitTimeout"
		go func() {
			select {
			case <-time.After(remains):
				if err = cmd.Process.Kill(); err != nil {
					log.Errorf("failed to kill: ", err)
				}
				log.Warningf("process killed as timeout reached")
			case err = <-done:
				if err != nil {
					log.Warning(err)
				}
			}
		}()
	case err = <-done:
		if err != nil {
			log.Warningf("process done with error = %v", err)
		} else {
			log.V(10).Infof("process done gracefully without error")
		}
	}

	if outstr != "" {
		return outstr, nil
	}

	output := out.String()
	log.V(10).Infof("output: %s, err: %v", output, err)
	return output, err
}
