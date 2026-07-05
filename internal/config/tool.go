package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/DeprecatedLuar/agentctl/internal/resolution"
)

const (
	// Directory and file names
	toolsDir        = "tools"
	exampleToolFile = "example.toml"
	tomlExtension   = ".toml"

	// Reserved TOML keys (not parameters)
	keyCommand     = "command"
	keyDescription = "description"

	// Default parameter type
	defaultParameterType = "string"
)

type ToolConfig struct {
	Name        string
	Command     string               `toml:"command"`
	Description string               `toml:"description"`
	Parameters  map[string]Parameter `toml:",inline"`
	Path        string               // Absolute path to the .toml file
}

type Parameter struct {
	Description string `toml:"description"`
	Type        string `toml:"type"`
	Required    bool   `toml:"required"`
	Enabled     bool   `toml:"enabled"`
	Return      string `toml:"return"` // Override value with directive support (hides from AI)
}

// shouldHideFromAI reports whether this parameter is excluded from the schema
// shown to the AI: either explicitly disabled, or given a return override that
// isn't a {{$completion}} placeholder (a blackbox value the AI never provides).
func (p Parameter) shouldHideFromAI() bool {
	return !p.Enabled || (p.Return != "" && !strings.Contains(p.Return, "{{$completion}}"))
}

func LoadTools(agentPath string, toolNames []string) ([]ToolConfig, []ValidationIssue) {
	var issues []ValidationIssue
	var tools []ToolConfig

	toolsPath := filepath.Join(agentPath, toolsDir)

	var filesToLoad []string

	if len(toolNames) == 0 {
		// Auto-discover: recursively load all .toml files except example.toml.
		// Walks manually (not filepath.WalkDir) because WalkDir never follows
		// symlinked directories - it reports them as leaf entries and skips
		// their contents, which breaks the common pattern of symlinking a
		// shared tool folder (e.g. tools/memory -> some/shared/library) into
		// an agent's tools/ directory.
		err := walkToolsDir(toolsPath, make(map[string]bool), &filesToLoad)
		if err != nil {
			issues = append(issues, ValidationIssue{
				Type:    IssueError,
				Message: fmt.Sprintf("tools/: %v", err),
			})
			return nil, issues
		}
	} else {
		// Load specified tools - validate they exist
		for _, name := range toolNames {
			filename := name
			if !strings.HasSuffix(filename, tomlExtension) {
				filename = name + tomlExtension
			}
			toolPath := filepath.Join(toolsPath, filename)
			if _, err := os.Stat(toolPath); err != nil {
				issues = append(issues, ValidationIssue{
					Type:    IssueError,
					Message: fmt.Sprintf("tools/%s: declared in agent.toml but file not found", filename),
				})
				continue
			}
			filesToLoad = append(filesToLoad, toolPath)
		}
	}

	for _, toolPath := range filesToLoad {
		tool, toolIssues := loadTool(toolPath, toolsPath)
		issues = append(issues, toolIssues...)

		// Only add tool if no errors occurred
		hasError := false
		for _, issue := range toolIssues {
			if issue.Type == IssueError {
				hasError = true
				break
			}
		}
		if !hasError {
			tools = append(tools, tool)
		}
	}

	return tools, issues
}

// walkToolsDir recursively collects .toml files under root, following
// symlinked directories (unlike filepath.WalkDir). visited tracks resolved
// real paths already walked, to guard against symlink cycles.
func walkToolsDir(root string, visited map[string]bool, filesToLoad *[]string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if visited[realRoot] {
		return nil
	}
	visited[realRoot] = true

	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())

		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(path) // follows the symlink
			if err != nil {
				continue // broken symlink, skip
			}
			isDir = info.IsDir()
		}

		if isDir {
			if err := walkToolsDir(path, visited, filesToLoad); err != nil {
				return err
			}
			continue
		}

		if entry.Name() == exampleToolFile {
			continue
		}
		if strings.HasSuffix(entry.Name(), tomlExtension) {
			*filesToLoad = append(*filesToLoad, path)
		}
	}

	return nil
}

func loadTool(path string, toolsBasePath string) (ToolConfig, []ValidationIssue) {
	var issues []ValidationIssue

	// Extract filename and compute relative path for error messages
	filename := filepath.Base(path)
	relPath := strings.TrimPrefix(path, toolsBasePath)
	relPath = strings.TrimPrefix(relPath, string(filepath.Separator)) // Remove leading separator
	if relPath == "" {
		relPath = filename
	}

	data, err := os.ReadFile(path)
	if err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: %v", relPath, err),
		})
		return ToolConfig{}, issues
	}

	// Parse as generic map to handle dynamic parameter sections
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: failed to parse TOML: %v", relPath, err),
		})
		return ToolConfig{}, issues
	}

	tool := ToolConfig{
		Name:       strings.TrimSuffix(filename, tomlExtension),
		Path:       path, // Store absolute path
		Parameters: make(map[string]Parameter),
	}

	// Extract command and description
	if cmd, ok := raw[keyCommand].(string); ok {
		tool.Command = cmd
	}
	if desc, ok := raw[keyDescription].(string); ok {
		tool.Description = desc
	}

	// Validation
	if tool.Command == "" {
		issues = append(issues, ValidationIssue{
			Type:    IssueError,
			Message: fmt.Sprintf("tools/%s: command field is required", relPath),
		})
	}

	// Extract parameters (all other sections are parameters)
	for key, value := range raw {
		if key == keyCommand || key == keyDescription {
			continue
		}

		paramMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		param := Parameter{
			Type:    defaultParameterType, // default
			Enabled: true,                 // default enabled
		}

		if typ, ok := paramMap["type"].(string); ok {
			param.Type = typ
		}
		if req, ok := paramMap["required"].(bool); ok {
			param.Required = req
		}
		if enabled, ok := paramMap["enabled"].(bool); ok {
			param.Enabled = enabled
		}
		if ret, ok := paramMap["return"].(string); ok {
			param.Return = ret

			// Validate directive syntax in return field (syntax only, no execution)
			if ret != "" {
				if err := resolution.ValidateSyntax(ret); err != nil {
					issues = append(issues, ValidationIssue{
						Type:    IssueError,
						Message: fmt.Sprintf("tools/%s: parameter '%s' return field: %v", relPath, key, err),
					})
				}
			}
		}

		if desc, ok := paramMap["description"].(string); ok {
			param.Description = desc
		} else if !param.shouldHideFromAI() {
			// Warn if parameter missing description, unless it's hidden from the AI anyway
			issues = append(issues, ValidationIssue{
				Type:    IssueWarning,
				Message: fmt.Sprintf("tools/%s: parameter '%s' missing description", relPath, key),
			})
		}

		tool.Parameters[key] = param
	}

	return tool, issues
}

// ConvertToolParameters converts tool parameters to OpenAI-compatible schema format.
// Filters out disabled parameters and parameters with return overrides that don't use {{$completion}}.
// Parameters with {{$completion}} in return field are shown to AI (AI provides value, we format it).
// Returns properties map and required parameter names slice.
func ConvertToolParameters(tool *ToolConfig) (map[string]interface{}, []string) {
	properties := make(map[string]interface{})
	required := []string{}

	for name, param := range tool.Parameters {
		if param.shouldHideFromAI() {
			continue
		}

		properties[name] = map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Required {
			required = append(required, name)
		}
	}

	return properties, required
}
