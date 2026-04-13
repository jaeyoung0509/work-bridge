# Switcher and Importer Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the codebase to eliminate hardcoded switch statements by introducing Interfaces/Registries, and vastly improve performance using goroutines and channels for concurrent I/O operations.

**Architecture:** We will introduce a `ToolImporter` interface with a registry in `internal/importer` to adhere to the Open-Closed Principle. We will rewrite the file reading logic to use worker pools for `readInstructionArtifacts` and `readSettingsSnapshot`. Then we'll break down the `projectAdapter` God Object into specialized components (`ToolLocator`, `SessionWriter`, `SkillWriter`, `MCPWriter`) and execute their I/O operations concurrently using `errgroup`.

**Tech Stack:** Go (goroutines, channels, `golang.org/x/sync/errgroup`)

---

### Task 1: Introduce ToolImporter Interface and Registry

**Files:**
- Create: `internal/importer/registry.go`
- Modify: `internal/importer/importer.go`
- Modify: `internal/importer/claude.go`
- Modify: `internal/importer/gemini.go`
- Modify: `internal/importer/codex.go`
- Modify: `internal/importer/opencode.go`

- [ ] **Step 1: Define interface and registry**

Create `internal/importer/registry.go`:
```go
package importer

import (
	"fmt"
	"sync"
)

type ToolImporter interface {
	ImportRaw(opts Options) (RawImportResult, error)
}

var (
	importersMu sync.RWMutex
	importers   = make(map[string]ToolImporter)
)

func Register(tool string, importer ToolImporter) {
	importersMu.Lock()
	defer importersMu.Unlock()
	if importer == nil {
		panic("importer: Register importer is nil")
	}
	if _, dup := importers[tool]; dup {
		panic("importer: Register called twice for tool " + tool)
	}
	importers[tool] = importer
}

func getImporter(tool string) (ToolImporter, error) {
	importersMu.RLock()
	defer importersMu.RUnlock()
	importer, ok := importers[tool]
	if !ok {
		return nil, fmt.Errorf("unsupported tool %q", tool)
	}
	return importer, nil
}
```

- [ ] **Step 2: Update ImportRaw to use registry**

Modify `internal/importer/importer.go`:
```go
func ImportRaw(opts Options) (RawImportResult, error) {
	imp, err := getImporter(opts.Tool)
	if err != nil {
		return RawImportResult{}, err
	}
	return imp.ImportRaw(opts)
}
```
*(Remove the switch statement for tools).*

- [ ] **Step 3: Implement interface in each tool importer**

Modify `claude.go` (and apply similarly to `gemini.go`, `codex.go`, `opencode.go`):
```go
func init() {
	Register("claude", &claudeImporter{})
}

type claudeImporter struct{}

func (i *claudeImporter) ImportRaw(opts Options) (RawImportResult, error) {
	// Rename importClaude logic to here
```

- [ ] **Step 4: Verify Compilation and Tests**

Run: `go test ./internal/importer/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/importer
git commit -m "refactor: introduce ToolImporter interface and registry"
```

### Task 2: Parallelize Importer File Reading

**Files:**
- Modify: `internal/importer/importer.go`

- [ ] **Step 1: Parallelize readInstructionArtifacts**

Modify `readInstructionArtifacts` in `internal/importer/importer.go` to use `errgroup` and a mutex for collecting results:
```go
import (
	"golang.org/x/sync/errgroup"
	"sync"
)

func readInstructionArtifacts(fs fsx.FS, tool domain.Tool, assets []detect.ArtifactProbe) []domain.InstructionArtifact {
	var artifacts []domain.InstructionArtifact
	var mu sync.Mutex
	var g errgroup.Group

	for _, asset := range assets {
		if asset.Kind != "instruction" || !asset.Found {
			continue
		}
		a := asset // capture loop variable
		g.Go(func() error {
			data, err := fs.ReadFile(a.Path)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256(data)
			art := domain.InstructionArtifact{
				Tool:        tool,
				Kind:        "project_instruction",
				Path:        a.Path,
				Scope:       a.Scope,
				Content:     string(data),
				ContentHash: hex.EncodeToString(sum[:]),
			}
			mu.Lock()
			artifacts = append(artifacts, art)
			mu.Unlock()
			return nil
		})
	}
	g.Wait()
	return artifacts
}
```

- [ ] **Step 2: Parallelize readSettingsSnapshot**

Modify `readSettingsSnapshot` using the same `errgroup` and mutex approach for concurrency.

- [ ] **Step 3: Verify tests**

Run: `go test ./internal/importer/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/importer/importer.go
git commit -m "perf: parallelize file reading in importer"
```

### Task 3: Extract ToolLocator Interface

**Files:**
- Create: `internal/switcher/locator.go`
- Modify: `internal/switcher/apply.go`

- [ ] **Step 1: Define ToolLocator**

Create `internal/switcher/locator.go`:
```go
package switcher

import (
	"path/filepath"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

type ToolLocator interface {
	InstructionPath(projectRoot string) string
	ConfigPath(projectRoot string) string
	ProjectSkillRoot(destinationRoot string) string
}

func getLocator(target domain.Tool) ToolLocator {
	switch target {
	case domain.ToolClaude:
		return &claudeLocator{}
	case domain.ToolGemini:
		return &geminiLocator{}
	case domain.ToolCodex:
		return &codexLocator{}
	case domain.ToolOpenCode:
		return &opencodeLocator{}
	default:
		return &defaultLocator{}
	}
}

type claudeLocator struct{}
func (l *claudeLocator) InstructionPath(root string) string { return filepath.Join(root, "CLAUDE.md") }
func (l *claudeLocator) ConfigPath(root string) string { return filepath.Join(root, ".claude", "settings.local.json") }
func (l *claudeLocator) ProjectSkillRoot(root string) string { return filepath.Join(root, ".claude", "skills") }

// Similar implementations for geminiLocator, codexLocator, opencodeLocator...
```

- [ ] **Step 2: Remove hardcoded paths from projectAdapter**

Remove `instructionPath`, `configPath`, `projectSkillRoot` from `projectAdapter` in `internal/switcher/apply.go`. Update all calls to use `getLocator(a.target)`.

- [ ] **Step 3: Verify tests**

Run: `go test ./internal/switcher/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/switcher
git commit -m "refactor: extract ToolLocator to manage target specific paths"
```

### Task 4: Parallelize File Writing in applyPlan

**Files:**
- Modify: `internal/switcher/apply.go`

- [ ] **Step 1: Use errgroup in applyPlan**

Modify `applyPlan` in `internal/switcher/apply.go` to run write functions concurrently.

```go
func (a *projectAdapter) applyPlan(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	// Setup report struct...
	var mu sync.Mutex
	var g errgroup.Group

	// Write session artifacts
	g.Go(func() error {
		exportManifest, changed, backups, err := a.writeSessionArtifacts(payload.Bundle, plan)
		if err != nil { return err }
		mu.Lock()
		defer mu.Unlock()
		report.FilesUpdated = append(report.FilesUpdated, changed...)
		report.BackupsCreated = append(report.BackupsCreated, backups...)
		report.Session.Files = append(report.Session.Files, changed...)
		report.Session.Summary = fmt.Sprintf("%d session files applied", len(exportManifest.Files))
		return nil
	})

	// Write skills
	g.Go(func() error {
		skillChanged, skillBackups, skillWarnings, err := a.writeSkills(payload, plan)
		if err != nil { return err }
		mu.Lock()
		defer mu.Unlock()
		report.FilesUpdated = append(report.FilesUpdated, skillChanged...)
		report.BackupsCreated = append(report.BackupsCreated, skillBackups...)
		report.Skills.Files = append(report.Skills.Files, skillChanged...)
		report.Warnings = append(report.Warnings, skillWarnings...)
		report.Skills.Summary = fmt.Sprintf("%d skill files applied", len(skillChanged))
		return nil
	})

	// Write MCP
	g.Go(func() error {
		// ... same pattern
		return nil
	})

	if err := g.Wait(); err != nil {
		return report, err
	}

	// Write instruction file after others to ensure consistency
	// dedupe and aggregate status...
	return report, nil
}
```

- [ ] **Step 2: Verify tests**

Run: `go test ./internal/switcher/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/switcher/apply.go
git commit -m "perf: parallelize I/O operations in applyPlan using errgroup"
```

### Task 5: Parallelize Skill Files Copying

**Files:**
- Modify: `internal/switcher/apply.go`

- [ ] **Step 1: Refactor writeSkillBundle**

Modify `writeSkillBundle` to copy multiple files inside a skill concurrently using `errgroup`:

```go
func (a *projectAdapter) writeSkillBundle(targetDir string, skill domain.SkillPayload) ([]string, []string, error) {
	// ... validation ...
	var mu sync.Mutex
	updated := []string{}
	backups := []string{}
	var g errgroup.Group
	g.SetLimit(10) // Limit concurrency for file I/O

	for _, src := range skillFilesForPayload(skill) {
		srcFile := src // capture
		g.Go(func() error {
			rel, err := filepath.Rel(skill.RootPath, srcFile)
			// ... error checking
			data, err := a.fs.ReadFile(srcFile)
			if err != nil { return err }
			
			changed, backup, err := a.writeFile(filepath.Join(targetDir, rel), string(data))
			if err != nil { return err }
			
			mu.Lock()
			if changed { updated = append(updated, filepath.Join(targetDir, rel)) }
			if backup != "" { backups = append(backups, backup) }
			mu.Unlock()
			
			return nil
		})
	}
	
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	return dedupeStrings(updated), dedupeStrings(backups), nil
}
```

- [ ] **Step 2: Verify tests**

Run: `go test ./internal/switcher/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/switcher/apply.go
git commit -m "perf: parallelize file copying in writeSkillBundle"
```
