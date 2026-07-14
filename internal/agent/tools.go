package agent

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
	"github.com/DeprecatedLuar/agentctl/internal/shell"
)

const (
	// Placeholder format
	placeholderFormat = "{{%s}}"

	// Error format
	exitCodeFormat = "exit %d: %s"

	// Output preview length shown in the [TOOL] log line (full output shown under -v)
	toolPreviewLen = 80

	// Output preview length shown in a tool report's {{$result}} token
	reportPreviewLen = 200

	// Tools directly under tools/ (no subfolder) get no family prefix
	topLevelToolsDir = "tools"
)

// ExecResult carries a tool's execution outcome for both the agentic loop
// (tool-result message) and report resolution ({{$command}}/{{$result}}).
type ExecResult struct {
	Output        string            // stdout, or "exit N: stderr" on error
	Command       string            // resolved command sent to shell
	Substitutions map[string]string // resolved param values
}

// ExecuteTool runs a tool with the given arguments
func ExecuteTool(tool *config.ToolConfig, args map[string]interface{}, agentFolder string, runtimeCtx resolution.Context, lg *slog.Logger, verbose bool, debug bool) ExecResult {
	// Build substitution map: Process return overrides with directive support
	substitutions := make(map[string]string)

	// First, add AI-provided arguments
	for key, value := range args {
		substitutions[key] = strings.TrimSpace(fmt.Sprintf("%v", value))
	}

	// Then, process return overrides (takes precedence over AI args)
	for paramName, param := range tool.Parameters {
		// Hard disable: if parameter is disabled, don't use it at all (even if return is set)
		if !param.Enabled {
			continue
		}

		// Process return override if set
		if param.Return != "" {
			// Process directives and variables in return value (uses full runtime context)
			processedValue, err := resolution.Process(param.Return, runtimeCtx)
			if err != nil {
				// Directive processing failed - return formatted error (match tool error format)
				return ExecResult{Output: fmt.Sprintf(exitCodeFormat, 1, fmt.Sprintf("return directive failed for '%s': %v", paramName, err))}
			}

			// Handle {{$completion}} placeholder
			if strings.Contains(processedValue, "{{$completion}}") {
				aiValue := ""
				if val, exists := args[paramName]; exists {
					aiValue = strings.TrimSpace(fmt.Sprintf("%v", val))
				}
				if aiValue == "" {
					processedValue = "" // Empty optional param
				} else {
					processedValue = strings.ReplaceAll(processedValue, "{{$completion}}", aiValue)
				}
			}

			substitutions[paramName] = strings.TrimSpace(processedValue)
		}
	}

	// Substitute variables in the command
	cmd := tool.Command
	for key, value := range substitutions {
		placeholder := fmt.Sprintf(placeholderFormat, key)
		cmd = strings.ReplaceAll(cmd, placeholder, value)
	}

	// Build tool environment: inject all resolved params as TOOL_<PARAMNAME> env vars.
	// A tool that needs system context (user, session, etc.) declares a hidden
	// param with return = "{{$var}}" rather than getting it injected ambiently.
	// A param can override its injected name via `env = "NAME"` (e.g. to match
	// an env var a wrapped script/CLI expects verbatim, like GH_TOKEN) - this
	// replaces the TOOL_ binding rather than adding to it.
	toolEnv := make(map[string]string)
	for key, value := range substitutions {
		envKey := "TOOL_" + strings.ToUpper(key)
		if param, ok := tool.Parameters[key]; ok && param.Env != "" {
			envKey = param.Env
		}
		toolEnv[envKey] = value
	}

	// Determine working directory
	// Tools run in their own directory (so they can reference local files with ./)
	// Falls back to agent folder if tool has no path (shouldn't happen in practice)
	workDir := agentFolder
	if tool.Path != "" {
		workDir = filepath.Dir(tool.Path)
	}

	// Execute via shell (workDir for command execution, agentFolder for .env loading)
	stdout, stderr, exitCode, err := shell.Execute(cmd, workDir, agentFolder, runtimeCtx.ConfigEnv, toolEnv)

	// Format result
	result := stdout
	if err != nil {
		result = fmt.Sprintf(exitCodeFormat, exitCode, stderr)
	}

	// Log tool execution as a single line: name, resolved args, exit code,
	// output preview. Raw shell command only shown under --debug. A non-zero
	// exit logs at Warn (no kind) so failures keep their [WARN] tag instead
	// of the routine [TOOL] one.
	if lg != nil {
		out := stdout
		if !verbose {
			out = truncate(out, toolPreviewLen)
		}

		fields := []any{"kind", logger.KindTool, "args", formatArgs(substitutions)}
		if debug {
			fields = append(fields, "command", cmd)
		}
		fields = append(fields, "out", out, "exit", exitCode)

		if exitCode != 0 {
			fields = append(fields, "stderr", stderr)
			lg.Warn(tool.Name, fields...)
		} else {
			lg.Info(tool.Name, fields...)
		}
	}

	return ExecResult{Output: result, Command: cmd, Substitutions: substitutions}
}

// ToolReport is a tool-use report split into its family and message parts,
// so each gateway can render them in its own style (e.g. "[family] message"
// vs "family: message") instead of receiving a pre-formatted string.
type ToolReport struct {
	Family  string // Tool's parent folder name, or the tool's own name for tools directly under tools/
	Message string
}

// ResolveToolReport renders a tool's report template for delivery to the
// originating interface. Returns a zero ToolReport if the tool has no report
// configured. {{$command}} and {{$result}} are local tokens resolved after
// resolution.Process runs - they never enter the global sysvar namespace.
func ResolveToolReport(tool *config.ToolConfig, exec ExecResult, runtimeCtx resolution.Context) (ToolReport, error) {
	if tool.Report == "" {
		return ToolReport{}, nil
	}

	s, err := resolution.Process(tool.Report, runtimeCtx)
	if err != nil {
		return ToolReport{}, err
	}

	s = strings.ReplaceAll(s, "{{$command}}", exec.Command)
	s = strings.ReplaceAll(s, "{{$result}}", truncate(exec.Output, reportPreviewLen))

	for key, value := range exec.Substitutions {
		placeholder := fmt.Sprintf(placeholderFormat, key)
		s = strings.ReplaceAll(s, placeholder, value)
	}

	family := filepath.Base(filepath.Dir(tool.Path))
	if family == topLevelToolsDir {
		family = tool.Name
	}

	return ToolReport{Family: family, Message: s}, nil
}

// formatArgs renders resolved tool args as a stable, readable string for logging.
func formatArgs(substitutions map[string]string) string {
	if len(substitutions) == 0 {
		return ""
	}

	keys := make([]string, 0, len(substitutions))
	for k := range substitutions {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%q", k, substitutions[k])
	}
	return strings.Join(parts, " ")
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FindTool finds a tool by name in the tools list
func FindTool(tools []config.ToolConfig, name string) *config.ToolConfig {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}
