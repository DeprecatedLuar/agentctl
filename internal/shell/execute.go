package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

const (
	shellCmd     = "sh"
	shellCmdFlag = "-c"
)

// Execute runs a shell command in the specified directory
// Returns stdout, stderr, exit code, and any execution error
// This is a pure function with no logging or formatting logic
// Environment precedence: process env < configEnv < .env < toolEnv
// workDir is where the command runs, agentFolder is where .env is loaded from
func Execute(cmd string, workDir string, agentFolder string, configEnv map[string]string, toolEnv map[string]string) (stdout, stderr string, exitCode int, err error) {
	execCmd := exec.Command(shellCmd, shellCmdFlag, cmd)
	execCmd.Dir = workDir

	// Build environment map with precedence: process env < configEnv < .env < toolEnv
	envMap := make(map[string]string)

	// 1. Start with current process environment (lowest priority)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// 2. Apply config environment (overrides process env)
	for k, v := range configEnv {
		envMap[k] = v
	}

	// 3. Load .env from agent folder (overrides config env)
	envPath := filepath.Join(agentFolder, ".env")
	dotenvMap, _ := godotenv.Read(envPath) // Ignore error - file is optional
	for k, v := range dotenvMap {
		envMap[k] = v
	}

	// 4. Set AGENT_PATH for tools to access agent root (always available)
	envMap["AGENT_PATH"] = agentFolder

	// 5. Apply tool-specific environment (highest priority, overrides everything)
	for k, v := range toolEnv {
		envMap[k] = v
	}

	// Convert map to []string for cmd.Env
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	execCmd.Env = env

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
