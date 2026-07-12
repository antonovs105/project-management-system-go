package coveragegate

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type coverageBlock struct {
	file       string
	statements int
	covered    bool
}

type coverageTotals struct {
	statements int
	covered    int
}

// TestRiskBasedCoverageFloors merges unit and integration coverage and protects
// the business-critical packages from regressions hidden by an aggregate floor.
func TestRiskBasedCoverageFloors(t *testing.T) {
	rawProfiles := strings.TrimSpace(os.Getenv("COVERAGE_PROFILES"))
	if rawProfiles == "" {
		t.Skip("set COVERAGE_PROFILES to enforce merged coverage floors")
	}

	blocks := make(map[string]coverageBlock)
	for _, profile := range strings.Split(rawProfiles, ",") {
		profile = strings.TrimSpace(profile)
		if profile == "" {
			continue
		}
		mergeProfile(t, profile, blocks)
	}
	if len(blocks) == 0 {
		t.Fatal("coverage profiles contained no statement blocks")
	}

	aggregate := coverageTotals{}
	packages := make(map[string]coverageTotals)
	for _, block := range blocks {
		aggregate.add(block)
		packageName := packageNameFor(block.file)
		totals := packages[packageName]
		totals.add(block)
		packages[packageName] = totals
	}

	assertFloor(t, "aggregate", aggregate, 50)
	for packageName, floor := range map[string]float64{
		"internal/project":      45,
		"internal/ticket":       45,
		"internal/comment":      45,
		"internal/notification": 50,
	} {
		assertFloor(t, packageName, packages[packageName], floor)
	}
}

func (totals *coverageTotals) add(block coverageBlock) {
	totals.statements += block.statements
	if block.covered {
		totals.covered += block.statements
	}
}

func assertFloor(t *testing.T, name string, totals coverageTotals, floor float64) {
	t.Helper()
	if totals.statements == 0 {
		t.Fatalf("%s coverage has no statements", name)
	}
	percentage := float64(totals.covered) * 100 / float64(totals.statements)
	t.Logf("%s coverage: %.1f%% (%d/%d statements)", name, percentage, totals.covered, totals.statements)
	if percentage < floor {
		t.Errorf("%s coverage %.1f%% is below %.1f%% floor", name, percentage, floor)
	}
}

func mergeProfile(t *testing.T, path string, blocks map[string]coverageBlock) {
	t.Helper()
	path = resolveProfilePath(t, path)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open coverage profile %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("invalid coverage line in %s: %q", path, line)
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("invalid statement count in %s: %q", path, fields[1])
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			t.Fatalf("invalid execution count in %s: %q", path, fields[2])
		}
		location := fields[0]
		fileName, _, ok := strings.Cut(location, ":")
		if !ok {
			t.Fatalf("invalid coverage location in %s: %q", path, location)
		}
		key := fmt.Sprintf("%s %d", location, statements)
		current, exists := blocks[key]
		if !exists {
			blocks[key] = coverageBlock{file: filepath.ToSlash(fileName), statements: statements, covered: count > 0}
			continue
		}
		current.covered = current.covered || count > 0
		blocks[key] = current
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read coverage profile %s: %v", path, err)
	}
}

func resolveProfilePath(t *testing.T, path string) string {
	t.Helper()
	if filepath.IsAbs(path) {
		return path
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("get coverage gate working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return filepath.Join(root, path)
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("find module root for coverage profile %s", path)
		}
		root = parent
	}
}

func packageNameFor(fileName string) string {
	const marker = "/internal/"
	index := strings.Index(fileName, marker)
	if index == -1 {
		return strings.TrimSuffix(fileName, "/"+filepath.Base(fileName))
	}
	relative := fileName[index+1:]
	return strings.TrimSuffix(relative, "/"+filepath.Base(relative))
}
