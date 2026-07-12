// Package architecture contains backend architecture boundary tests.
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const modulePath = "github.com/antonovs105/project-management-system-go"

// TestBackendArchitectureBoundaries guards the main vertical-slice boundaries.
func TestBackendArchitectureBoundaries(t *testing.T) {
	moduleRoot := backendModuleRoot(t)
	var violations []string

	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".cache", ".gocache", ".gotmp", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		violations = append(violations, fileBoundaryViolations(moduleRoot, path, file)...)
		return nil
	})
	if err != nil {
		t.Fatalf("scan backend architecture: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("backend architecture boundary violations:\n%s", strings.Join(violations, "\n"))
	}
}

// backendModuleRoot returns the backend module path from this test package.
func backendModuleRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// fileBoundaryViolations checks one production Go file for forbidden dependencies.
func fileBoundaryViolations(moduleRoot, path string, file *ast.File) []string {
	relPath, err := filepath.Rel(moduleRoot, path)
	if err != nil {
		relPath = path
	}
	relPath = filepath.ToSlash(relPath)

	var violations []string
	for _, imported := range file.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		switch {
		case strings.HasPrefix(importPath, modulePath+"/cmd/"):
			violations = append(violations, relPath+": internal packages must not import command packages")
		case importPath == "github.com/labstack/echo/v4" && !isHTTPAdapterFile(relPath):
			violations = append(violations, relPath+": Echo must stay in HTTP adapter files")
		case isForbiddenActivityPubImport(relPath, importPath):
			violations = append(violations, relPath+": ActivityPub core must not import project-management slices")
		}
	}
	return violations
}

// isHTTPAdapterFile reports whether a file is allowed to depend on Echo.
func isHTTPAdapterFile(relPath string) bool {
	return relPath == "cmd/api/main.go" ||
		strings.HasPrefix(relPath, "internal/middleware/") ||
		strings.HasPrefix(relPath, "internal/apiresponse/") ||
		strings.HasSuffix(relPath, "/handler.go")
}

// isForbiddenActivityPubImport reports forbidden dependencies from ActivityPub core to feature slices.
func isForbiddenActivityPubImport(relPath string, importPath string) bool {
	if !strings.HasPrefix(relPath, "internal/activitypub/") {
		return false
	}
	if strings.HasPrefix(relPath, "internal/activitypub/c2s/") ||
		strings.HasPrefix(relPath, "internal/activitypub/moderation/") {
		return false
	}

	for _, featurePath := range []string{
		modulePath + "/internal/adminaudit",
		modulePath + "/internal/comment",
		modulePath + "/internal/project",
		modulePath + "/internal/ticket",
		modulePath + "/internal/user",
		modulePath + "/internal/webfinger",
	} {
		if importPath == featurePath {
			return true
		}
	}
	return false
}
