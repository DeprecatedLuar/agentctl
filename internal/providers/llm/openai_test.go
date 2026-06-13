package llm

import (
	"testing"

	"github.com/DeprecatedLuar/agentctl/internal/config"
)

// TestParameterFiltering validates that disabled parameters and parameters
// with return overrides are correctly excluded from tool schemas sent to AI providers.
func TestParameterFiltering(t *testing.T) {
	tool := &config.ToolConfig{
		Name:        "test_tool",
		Command:     "echo test",
		Description: "Test tool",
		Parameters: map[string]config.Parameter{
			"enabled": {
				Description: "Enabled parameter",
				Type:        "string",
				Required:    true,
				Enabled:     true,
			},
			"disabled": {
				Description: "Disabled parameter (blackbox to AI)",
				Type:        "string",
				Required:    false,
				Enabled:     false,
			},
			"with_return": {
				Description: "Parameter with return override (blackbox to AI)",
				Type:        "string",
				Required:    false,
				Enabled:     true,
				Return:      "hardcoded-value",
			},
			"disabled_with_return": {
				Description: "Disabled AND return (hard disable wins)",
				Type:        "string",
				Required:    false,
				Enabled:     false,
				Return:      "should-not-appear",
			},
			"default": {
				Description: "Default parameter",
				Type:        "string",
				Required:    false,
				Enabled:     true,
			},
		},
	}

	// Use the shared converter function
	properties, required := config.ConvertToolParameters(tool)

	// Should have 2 properties: "enabled" and "default"
	// Filtered out: "disabled" (enabled=false), "with_return" (has return), "disabled_with_return" (enabled=false)
	if len(properties) != 2 {
		t.Errorf("expected 2 properties, got %d", len(properties))
	}

	if _, ok := properties["enabled"]; !ok {
		t.Error("enabled parameter should be present")
	}

	if _, ok := properties["disabled"]; ok {
		t.Error("disabled parameter should NOT be present (it's disabled)")
	}

	if _, ok := properties["with_return"]; ok {
		t.Error("with_return parameter should NOT be present (has return override)")
	}

	if _, ok := properties["disabled_with_return"]; ok {
		t.Error("disabled_with_return parameter should NOT be present (disabled)")
	}

	if _, ok := properties["default"]; !ok {
		t.Error("default parameter should be present")
	}

	// Verify only enabled parameter is in required array
	if len(required) != 1 || required[0] != "enabled" {
		t.Errorf("expected required=['enabled'], got %v", required)
	}
}
