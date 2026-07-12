package agent

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/DeprecatedLuar/agentctl/internal/shell"
)

// ExecuteRoutine runs a routine's command on a scheduler fire. Mirrors
// ExecuteTool's substitution/injection pipeline (return overrides, directive
// processing, working dir from the routine's own file location), except
// params inject as ROUTINE_<NAME> instead of TOOL_<NAME> and there is no
// AI-provided argument set or {{$completion}} handling - a routine fires on
// a schedule, not a model tool-call, and its result is fire-and-forget.
func ExecuteRoutine(routine *config.RoutineConfig, agentFolder string, runtimeCtx resolution.Context, lg *slog.Logger, debug bool) {
	substitutions := make(map[string]string)

	for paramName, param := range routine.Parameters {
		if !param.Enabled || param.Return == "" {
			continue
		}
		processedValue, err := resolution.Process(param.Return, runtimeCtx)
		if err != nil {
			if lg != nil {
				lg.Warn(routine.Name, "kind", logger.KindTool, "error", fmt.Sprintf("return directive failed for '%s': %v", paramName, err))
			}
			return
		}
		substitutions[paramName] = strings.TrimSpace(processedValue)
	}

	cmd := routine.Command
	for key, value := range substitutions {
		placeholder := fmt.Sprintf(placeholderFormat, key)
		cmd = strings.ReplaceAll(cmd, placeholder, value)
	}

	routineEnv := make(map[string]string)
	for key, value := range substitutions {
		envKey := "ROUTINE_" + strings.ToUpper(key)
		routineEnv[envKey] = value
	}

	workDir := agentFolder
	if routine.Path != "" {
		workDir = filepath.Dir(routine.Path)
	}

	stdout, stderr, exitCode, err := shell.Execute(cmd, workDir, agentFolder, runtimeCtx.ConfigEnv, routineEnv)

	if lg == nil {
		return
	}

	out := stdout
	if !debug {
		out = truncate(out, toolPreviewLen)
	}

	fields := []any{"kind", logger.KindTool, "args", formatArgs(substitutions)}
	if debug {
		fields = append(fields, "command", cmd)
	}
	fields = append(fields, "out", out, "exit", exitCode)

	if err != nil {
		fields = append(fields, "stderr", stderr)
		lg.Warn(routine.Name, fields...)
	} else {
		lg.Info(routine.Name, fields...)
	}
}
