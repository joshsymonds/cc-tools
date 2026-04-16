# Context Window Optimization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace wasteful transcript parsing with pre-calculated context_window data from Claude Code

**Architecture:** Remove transcript file parsing and use the `context_window.used_percentage` value directly from the JSON input. This eliminates file I/O and JSONL parsing overhead. The context bar rendering logic stays the same but consumes the percentage directly instead of calculating it.

**Tech Stack:** Go, existing statusline package

---

## Task 1: Update Input struct with ContextWindow field

**Files:**
- Modify: `internal/statusline/statusline.go:14-39`

**Step 1: Write the failing test**

Add test to `internal/statusline/statusline_test.go` that parses JSON with `context_window`:

```go
func TestInput_ContextWindow(t *testing.T) {
	jsonInput := `{
		"model": {"id": "claude-sonnet-4-5", "display_name": "Claude"},
		"context_window": {
			"used_percentage": 45.5,
			"context_window_size": 200000
		}
	}`

	var input Input
	err := json.Unmarshal([]byte(jsonInput), &input)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if input.ContextWindow.UsedPercentage != 45.5 {
		t.Errorf("Expected UsedPercentage 45.5, got %f", input.ContextWindow.UsedPercentage)
	}
	if input.ContextWindow.ContextWindowSize != 200000 {
		t.Errorf("Expected ContextWindowSize 200000, got %d", input.ContextWindow.ContextWindowSize)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/statusline -run TestInput_ContextWindow`
Expected: FAIL - ContextWindow field does not exist

**Step 3: Write minimal implementation**

Add to `Input` struct in `internal/statusline/statusline.go`:

```go
type Input struct {
	Model struct {
		ID          string `json:"id"`
		Provider    string `json:"provider"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Cost struct {
		TotalCostUSD     float64 `json:"total_cost_usd"`
		InputTokens      int     `json:"input_tokens"`
		OutputTokens     int     `json:"output_tokens"`
		CacheReadTokens  int     `json:"cache_read_input_tokens"`
		CacheWriteTokens int     `json:"cache_creation_input_tokens"`
	} `json:"cost"`
	ContextWindow struct {
		UsedPercentage    float64 `json:"used_percentage"`
		ContextWindowSize int     `json:"context_window_size"`
	} `json:"context_window"`
	GitInfo struct {
		Branch       string `json:"branch"`
		IsGitRepo    bool   `json:"is_git_repo"`
		HasUntracked bool   `json:"has_untracked"`
		HasModified  bool   `json:"has_modified"`
	} `json:"git_info"`
	Workspace struct {
		ProjectDir string `json:"project_dir"`
		CurrentDir string `json:"current_dir"`
		CWD        string `json:"cwd"`
	} `json:"workspace"`
	TranscriptPath string `json:"transcript_path"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/statusline -run TestInput_ContextWindow`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/statusline/statusline.go internal/statusline/statusline_test.go
git commit -m "$(cat <<'EOF'
feat(statusline): add ContextWindow field to Input struct

Prepare for receiving pre-calculated context window percentage from
Claude Code instead of parsing transcript files.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Update CachedData to store UsedPercentage

**Files:**
- Modify: `internal/statusline/statusline.go:49-65`

**Step 1: Write the failing test**

Add test in `internal/statusline/statusline_test.go`:

```go
func TestCachedData_UsedPercentage(t *testing.T) {
	data := &CachedData{
		UsedPercentage: 67.5,
	}

	if data.UsedPercentage != 67.5 {
		t.Errorf("Expected UsedPercentage 67.5, got %f", data.UsedPercentage)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/statusline -run TestCachedData_UsedPercentage`
Expected: FAIL - UsedPercentage field does not exist

**Step 3: Write minimal implementation**

Update `CachedData` struct - replace `ContextLength int` with `UsedPercentage float64`:

```go
type CachedData struct {
	ModelID        string
	ModelDisplay   string
	CurrentDir     string
	TranscriptPath string
	GitBranch      string
	GitStatus      string
	K8sContext     string
	InputTokens    int
	OutputTokens   int
	UsedPercentage float64  // Changed from ContextLength int
	Hostname       string
	Devspace       string
	DevspaceSymbol string
	TermWidth      int
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/statusline -run TestCachedData_UsedPercentage`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/statusline/statusline.go internal/statusline/statusline_test.go
git commit -m "$(cat <<'EOF'
refactor(statusline): replace ContextLength with UsedPercentage in CachedData

Store the percentage directly instead of raw token counts. This is
preparation for consuming pre-calculated percentages from Claude Code.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Update computeData to use input.ContextWindow.UsedPercentage

**Files:**
- Modify: `internal/statusline/statusline.go:214-268`

**Step 1: Write the failing test**

Add test in `internal/statusline/statusline_test.go`:

```go
func TestComputeData_UsesContextWindowPercentage(t *testing.T) {
	fr := NewMockFileReader()
	deps := &Dependencies{
		FileReader:    fr,
		CommandRunner: NewMockCommandRunner(),
		EnvReader:     NewMockEnvReader(),
		TerminalWidth: &MockTerminalWidth{width: 120},
	}

	s := CreateStatusline(deps)
	s.input = &Input{}
	s.input.ContextWindow.UsedPercentage = 42.5

	data := s.computeData("/home/user")

	if data.UsedPercentage != 42.5 {
		t.Errorf("Expected UsedPercentage 42.5, got %f", data.UsedPercentage)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/statusline -run TestComputeData_UsesContextWindowPercentage`
Expected: FAIL - computeData doesn't populate UsedPercentage from input

**Step 3: Write minimal implementation**

Update `computeData` function to use context window directly:

```go
func (s *Statusline) computeData(currentDir string) *CachedData {
	data := &CachedData{
		CurrentDir:     currentDir,
		TranscriptPath: s.input.TranscriptPath,
		TermWidth:      s.deps.TerminalWidth.GetWidth(),
		ModelID:        s.input.Model.ID,
	}

	// Model display name
	if s.input.Model.DisplayName != "" {
		data.ModelDisplay = s.input.Model.DisplayName
	} else {
		data.ModelDisplay = "Claude"
	}

	// Git information
	gitInfo := s.getGitInfo(currentDir)
	data.GitBranch = gitInfo.Branch
	data.GitStatus = gitInfo.Status

	// Kubernetes context
	data.K8sContext = s.getK8sContext()

	// Context window percentage (directly from input)
	data.UsedPercentage = s.input.ContextWindow.UsedPercentage

	// Hostname
	data.Hostname = s.getHostname()

	// Devspace
	data.Devspace, data.DevspaceSymbol = s.getDevspace()

	return data
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/statusline -run TestComputeData_UsesContextWindowPercentage`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/statusline/statusline.go internal/statusline/statusline_test.go
git commit -m "$(cat <<'EOF'
refactor(statusline): use ContextWindow.UsedPercentage in computeData

Consume the pre-calculated percentage from Claude Code input instead of
computing it from transcript files.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Update render.go to use UsedPercentage directly

**Files:**
- Modify: `internal/statusline/render.go:357-385`
- Modify: `internal/statusline/render.go:686-694`

**Step 1: Write the failing test**

Add test in `internal/statusline/context_bar_test.go`:

```go
func TestCreateContextBar_UsesPercentageDirectly(t *testing.T) {
	deps := &Dependencies{
		FileReader:    &MockFileReader{},
		CommandRunner: &MockCommandRunner{},
		EnvReader:     &MockEnvReader{vars: make(map[string]string)},
		TerminalWidth: &MockTerminalWidth{width: 100},
	}

	s := CreateStatusline(deps)
	data := &CachedData{
		UsedPercentage: 55.0,
		TermWidth:      100,
	}

	result := s.Render(data)

	// Should show context bar with approximately 55%
	if !strings.Contains(result, "55.0%") {
		t.Errorf("Expected context bar to show 55.0%%, got: %s", stripAnsi(result))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/statusline -run TestCreateContextBar_UsesPercentageDirectly`
Expected: FAIL - render doesn't use UsedPercentage, still uses ContextLength

**Step 3: Write minimal implementation**

Update `fillRemainingWidth` in render.go:

```go
func (s *Statusline) fillRemainingWidth(data *CachedData, width int) string {
	if width <= 0 {
		return ""
	}

	// Context bar only appears if there's at least 25 chars of space left after components
	// This ensures components get priority for space
	const minContextBarWidth = 25
	if data.UsedPercentage > 0 && width >= minContextBarWidth {
		return s.createContextBarFromPercentage(data.UsedPercentage, width)
	}

	// Otherwise just spaces
	return strings.Repeat(" ", width)
}
```

Add new method `createContextBarFromPercentage`:

```go
func (s *Statusline) createContextBarFromPercentage(percentage float64, barWidth int) string {
	availableForBar := s.calculateAvailableBarWidth(barWidth)
	const minSensibleBarSize = 15
	if availableForBar < minSensibleBarSize {
		return strings.Repeat(" ", barWidth)
	}

	bgColor, fgColor, fgLightBg := s.getContextColors(percentage)

	barInfo := s.prepareContextBarInfo(percentage, availableForBar)
	const minFillWidth = 4
	if barInfo.fillWidth < minFillWidth {
		return strings.Repeat(" ", barWidth)
	}

	s.debugContextBarInfo(barWidth, availableForBar, barInfo)

	progressBar := s.buildProgressBar(barInfo.fillWidth, barInfo.filledWidth, fgColor, fgLightBg)
	return s.assembleContextBar(barInfo, bgColor, fgColor, progressBar, barWidth)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/statusline -run TestCreateContextBar_UsesPercentageDirectly`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/statusline/render.go internal/statusline/context_bar_test.go
git commit -m "$(cat <<'EOF'
refactor(statusline): render context bar from UsedPercentage

Add createContextBarFromPercentage that takes percentage directly.
Update fillRemainingWidth to use UsedPercentage instead of ContextLength.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Delete getTokenMetrics function and related code

**Files:**
- Modify: `internal/statusline/statusline.go:368-435` (delete)
- Modify: `internal/statusline/statusline.go:41-47` (delete TokenMetrics struct)
- Modify: `internal/statusline/statusline.go:67-96` (delete ContextConfig and getContextConfig)

**Step 1: Verify no remaining usages**

Run: `grep -r "getTokenMetrics\|TokenMetrics\|getContextConfig\|ContextConfig" internal/statusline/*.go | grep -v "_test.go"`
Expected: Only the definitions themselves, no other usages after previous refactors

**Step 2: Delete unused code**

Remove from `internal/statusline/statusline.go`:
- `TokenMetrics` struct (lines 41-47)
- `ContextConfig` struct and `getContextConfig` function (lines 67-96)
- `getTokenMetrics` method (lines 368-435)

Remove from `internal/statusline/render.go`:
- `calculateContextPercentage` method (lines 686-694)
- `createContextBar` method (old version that takes contextLength and modelID)

**Step 3: Run all tests to verify nothing breaks**

Run: `go test -v ./internal/statusline/...`
Expected: All tests pass (some tests will need updating in next task)

**Step 4: Commit**

```bash
git add internal/statusline/statusline.go internal/statusline/render.go
git commit -m "$(cat <<'EOF'
refactor(statusline): remove transcript parsing code

Delete getTokenMetrics, TokenMetrics, ContextConfig, getContextConfig,
calculateContextPercentage, and old createContextBar. These are no
longer needed since percentage comes pre-calculated from Claude Code.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Update tests for new behavior

**Files:**
- Modify: `internal/statusline/statusline_extra_test.go` (delete TestStatusline_GetTokenMetrics_InvalidJSON)
- Modify: `internal/statusline/context_bar_test.go` (update to use UsedPercentage)
- Modify: `internal/statusline/spacer_test.go` (update CachedData to use UsedPercentage)

**Step 1: Delete obsolete tests**

Remove `TestStatusline_GetTokenMetrics_InvalidJSON` from `statusline_extra_test.go` (the entire test function).

**Step 2: Update context_bar_test.go**

Replace all `ContextLength: N` with `UsedPercentage: X.X` where X.X is the percentage:

```go
// Old: ContextLength: 50000 (which was 25% of 200k)
// New: UsedPercentage: 25.0
```

**Step 3: Update spacer_test.go**

Replace all `ContextLength: 50000` with `UsedPercentage: 25.0`

**Step 4: Run all tests**

Run: `go test -v ./internal/statusline/...`
Expected: All tests pass

**Step 5: Commit**

```bash
git add internal/statusline/statusline_extra_test.go internal/statusline/context_bar_test.go internal/statusline/spacer_test.go
git commit -m "$(cat <<'EOF'
test(statusline): update tests for UsedPercentage

Remove obsolete getTokenMetrics tests. Update context bar and spacer
tests to use UsedPercentage instead of ContextLength.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Run linter and fix issues

**Step 1: Run linter**

Run: `make lint`
Expected: May have issues with unused code or formatting

**Step 2: Fix any issues**

Apply `gofmt` and address any golangci-lint warnings.

**Step 3: Run tests again**

Run: `make test`
Expected: All tests pass

**Step 4: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
chore(statusline): fix lint issues

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Integration test with sample input

**Step 1: Build binaries**

Run: `make build`
Expected: Successful build

**Step 2: Test with sample JSON**

Run:
```bash
echo '{"cwd": "/home/user", "model": {"display_name": "Claude"}, "context_window": {"used_percentage": 35.5}}' | ./build/cc-tools-statusline
```

Expected: Statusline output with context bar showing approximately 35.5%

**Step 3: Verify no transcript parsing**

Ensure the statusline works even without a transcript_path field in the JSON.

**Step 4: Final commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat(statusline): complete context window optimization

Statusline now uses pre-calculated context_window.used_percentage from
Claude Code instead of parsing transcript files. This eliminates file
I/O and JSONL parsing overhead.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```
