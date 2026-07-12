package config

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEnvironmentVariableContractMatchesLoaderAndDeploymentFiles(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	wanted := EnvironmentVariables()
	sort.Strings(wanted)
	require.Equal(t, wanted, loaderEnvironmentVariables(t), "EnvironmentVariables must describe every applyEnv override")

	exampleVariables := dotenvVariables(t, filepath.Join(repoRoot, ".env.example"))
	for _, name := range wanted {
		require.Contains(t, exampleVariables, name, ".env.example must document %s", name)
	}

	composeFiles := []string{
		filepath.Join(repoRoot, "docker-compose.yml"),
		filepath.Join(repoRoot, "docker-compose.instance.yml"),
		filepath.Join(repoRoot, "deploy", "docker-compose.bluegreen.yml"),
	}
	for _, path := range composeFiles {
		for service, variables := range composeBackendEnvironments(t, path) {
			for _, name := range wanted {
				require.Contains(t, variables, name, "%s service %s must pass %s", filepath.Base(path), service, name)
			}
		}
	}
}

func TestExampleYAMLMatchesTypedSchema(t *testing.T) {
	path := filepath.Clean(filepath.Join("..", "..", "..", "progo.example.yml"))
	_, err := LoadFileNoEnv(path)
	require.NoError(t, err)
}

func loaderEnvironmentVariables(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile("config.go")
	require.NoError(t, err)
	pattern := regexp.MustCompile(`apply[A-Za-z]+Env\([^\n]*"([A-Z][A-Z0-9_]+)"\)`)
	matches := pattern.FindAllSubmatch(source, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, string(match[1]))
	}
	sort.Strings(values)
	return values
}

func dotenvVariables(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	values := make(map[string]struct{})
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, found := strings.Cut(line, "=")
		if found {
			values[strings.TrimSpace(name)] = struct{}{}
		}
	}
	return values
}

func composeBackendEnvironments(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var document struct {
		Services map[string]struct {
			Environment map[string]any `yaml:"environment"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal(content, &document))
	values := make(map[string]map[string]any)
	for name, service := range document.Services {
		if name == "backend" || name == "backend-worker" || strings.HasPrefix(name, "backend-") {
			values[name] = service.Environment
		}
	}
	require.NotEmpty(t, values, "%s must define backend services", filepath.Base(path))
	return values
}
