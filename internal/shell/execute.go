package shell

import (
	"bytes"
	"os/exec"
)

const (
	shellCmd     = "sh"
	shellCmdFlag = "-c"
)

// Execute runs a shell command in the specified directory
// Returns stdout, stderr, exit code, and any execution error
// This is a pure function with no logging or formatting logic
func Execute(cmd string, dir string) (stdout, stderr string, exitCode int, err error) {
	execCmd := exec.Command(shellCmd, shellCmdFlag, cmd)
	execCmd.Dir = dir

	var outBuf, errBuf bytes.Buffer
	execCmd.Stdout = &outBuf
	execCmd.Stderr = &errBuf

	err = execCmd.Run()

	stdout = outBuf.String()
	stderr = errBuf.String()
	exitCode = 0

	if err != nil {
		// Default exit code if we can't extract it
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return stdout, stderr, exitCode, err
}
