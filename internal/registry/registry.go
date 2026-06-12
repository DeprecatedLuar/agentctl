package registry

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	registryDir  = ".local/share/agentctl"
	registryFile = "agents"
)

// getRegistryPath returns the absolute path to the registry file
func getRegistryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, registryDir, registryFile), nil
}

// Register adds an agent path to the registry if not already present
func Register(path string) error {
	registryPath, err := getRegistryPath()
	if err != nil {
		return err
	}

	// Create directory if needed
	dir := filepath.Dir(registryPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	// Read existing entries
	existing := make(map[string]bool)
	if file, err := os.Open(registryPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				existing[line] = true
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read registry: %w", err)
		}
	}

	// Skip if already registered
	if existing[path] {
		return nil
	}

	// Append path
	file, err := os.OpenFile(registryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open registry: %w", err)
	}
	defer file.Close()

	if _, err := fmt.Fprintln(file, path); err != nil {
		return fmt.Errorf("failed to write to registry: %w", err)
	}

	return nil
}

// Resolve finds an agent path by name
func Resolve(name string) (string, error) {
	registryPath, err := getRegistryPath()
	if err != nil {
		return "", err
	}

	file, err := os.Open(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("agent '%s' not found in registry (is it initialized?)", name)
		}
		return "", fmt.Errorf("failed to read registry: %w", err)
	}
	defer file.Close()

	// Find all matching entries
	var matches []string
	var deadEntries []string
	var validLines []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Check if this path's base name matches
		if filepath.Base(line) == name {
			// Validate path exists
			if _, err := os.Stat(line); err == nil {
				matches = append(matches, line)
				validLines = append(validLines, line)
			} else {
				deadEntries = append(deadEntries, line)
			}
		} else {
			validLines = append(validLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read registry: %w", err)
	}

	// Auto-cleanup: rewrite registry if dead entries found
	if len(deadEntries) > 0 {
		if err := writeRegistry(registryPath, validLines); err != nil {
			// Log but don't fail - cleanup is best-effort
			fmt.Fprintf(os.Stderr, "Warning: failed to cleanup dead registry entries: %v\n", err)
		}
	}

	// Handle results
	if len(matches) == 0 {
		if len(deadEntries) > 0 {
			return "", fmt.Errorf("agent '%s' was registered but path no longer exists: %s", name, deadEntries[0])
		}
		return "", fmt.Errorf("agent '%s' not found in registry (is it initialized?)", name)
	}

	if len(matches) > 1 {
		return "", fmt.Errorf("multiple agents named '%s' found in registry:\n%s\nUse explicit path with -a flag", name, strings.Join(matches, "\n"))
	}

	return matches[0], nil
}

// List returns all registered agent paths
func List() ([]string, error) {
	registryPath, err := getRegistryPath()
	if err != nil {
		return nil, err
	}

	file, err := os.Open(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			paths = append(paths, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}

	return paths, nil
}

// writeRegistry overwrites the registry file with the given paths
func writeRegistry(registryPath string, paths []string) error {
	file, err := os.OpenFile(registryPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, path := range paths {
		if _, err := fmt.Fprintln(file, path); err != nil {
			return err
		}
	}

	return nil
}
