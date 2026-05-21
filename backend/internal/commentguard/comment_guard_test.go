// Package commentguard contains documentation quality gates for backend code.
package commentguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProductionDeclarationsHaveGoDoc ensures backend top-level declarations stay documented.
func TestProductionDeclarationsHaveGoDoc(t *testing.T) {
	moduleRoot := backendModuleRoot(t)
	var missing []string

	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".gocache", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			missing = append(missing, undocumentedDeclarations(fileSet, path, decl)...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan backend Go files: %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("top-level declarations need Go-doc comments:\n%s", strings.Join(missing, "\n"))
	}
}

// backendModuleRoot returns the backend module path from this test package.
func backendModuleRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate comment guard test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// undocumentedDeclarations reports top-level declarations whose comments do not start with their names.
func undocumentedDeclarations(fileSet *token.FileSet, path string, decl ast.Decl) []string {
	switch typed := decl.(type) {
	case *ast.FuncDecl:
		if hasGoDoc(typed.Name.Name, typed.Doc) {
			return nil
		}
		return []string{formatMissing(fileSet, path, typed.Pos(), typed.Name.Name)}
	case *ast.GenDecl:
		return undocumentedGenDecl(fileSet, path, typed)
	default:
		return nil
	}
}

// undocumentedGenDecl checks names declared by type, var, and const declarations.
func undocumentedGenDecl(fileSet *token.FileSet, path string, decl *ast.GenDecl) []string {
	var missing []string
	for _, spec := range decl.Specs {
		switch typed := spec.(type) {
		case *ast.TypeSpec:
			if !hasGoDoc(typed.Name.Name, firstDoc(typed.Doc, decl.Doc)) {
				missing = append(missing, formatMissing(fileSet, path, typed.Pos(), typed.Name.Name))
			}
		case *ast.ValueSpec:
			for _, name := range typed.Names {
				if !hasGoDoc(name.Name, firstDoc(typed.Doc, decl.Doc)) {
					missing = append(missing, formatMissing(fileSet, path, name.Pos(), name.Name))
				}
			}
		}
	}
	return missing
}

// firstDoc returns the nearest declaration comment group.
func firstDoc(groups ...*ast.CommentGroup) *ast.CommentGroup {
	for _, group := range groups {
		if group != nil {
			return group
		}
	}
	return nil
}

// hasGoDoc reports whether a comment follows the Go-doc name convention.
func hasGoDoc(name string, doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, comment := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(comment.Text, "//"), "/*"))
		if strings.HasPrefix(text, name+" ") ||
			strings.HasPrefix(text, name+".") ||
			strings.HasPrefix(text, name+",") ||
			strings.HasPrefix(text, name+" is") ||
			strings.HasPrefix(text, name+" are") {
			return true
		}
	}
	return false
}

// formatMissing renders a stable relative file location for a missing comment.
func formatMissing(fileSet *token.FileSet, path string, pos token.Pos, name string) string {
	location := fileSet.Position(pos)
	relPath, err := filepath.Rel(filepath.Dir(filepath.Dir(path)), path)
	if err != nil {
		relPath = path
	}
	return fmt.Sprintf("%s:%d: %s", filepath.ToSlash(relPath), location.Line, name)
}
