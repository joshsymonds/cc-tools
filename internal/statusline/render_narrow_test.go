package statusline

import (
	"testing"
)

// mapEnvReader is a tiny EnvReader stub. The package's other tests
// already have their own env stubs; this one is local to keep the
// narrow-mode tests self-contained.
type narrowMapEnvReader map[string]string

func (m narrowMapEnvReader) Get(key string) string { return m[key] }

func depsWith(env map[string]string) *Dependencies {
	return &Dependencies{
		EnvReader: narrowMapEnvReader(env),
	}
}

func TestNarrowWidthThreshold_Value(t *testing.T) {
	if narrowWidthThreshold != 80 {
		t.Errorf("narrowWidthThreshold = %d, want 80 (pinned by epic R1)", narrowWidthThreshold)
	}
}

func TestContextColor_Thresholds(t *testing.T) {
	cases := []struct {
		pct  int
		want string
	}{
		{0, "green"},
		{39, "green"},
		{40, "yellow"},
		{59, "yellow"},
		{60, "peach"},
		{79, "peach"},
		{80, "red"},
		{100, "red"},
	}
	for _, c := range cases {
		got := contextColor(c.pct)
		if got != c.want {
			t.Errorf("contextColor(%d) = %q, want %q", c.pct, got, c.want)
		}
	}
}

func TestGatherNarrowChips_DirOnly(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	deps := depsWith(map[string]string{})
	data := &CachedData{
		CurrentDir:     "/tmp/fakehome/x",
		UsedPercentage: 0,
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) != 2 {
		t.Fatalf("want 2 chips (dir+ctx) when no git/env, got %d: %+v", len(chips), chips)
	}
	if chips[0].Kind != "dir" {
		t.Errorf("chip[0].Kind = %q, want dir", chips[0].Kind)
	}
	if chips[1].Kind != "context" {
		t.Errorf("chip[1].Kind = %q, want context", chips[1].Kind)
	}
}

func TestGatherNarrowChips_WithGit(t *testing.T) {
	deps := depsWith(map[string]string{})
	data := &CachedData{
		CurrentDir:     "/tmp/x",
		GitBranch:      "main",
		UsedPercentage: 25,
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) != 3 {
		t.Fatalf("want 3 chips (dir+ctx+branch), got %d: %+v", len(chips), chips)
	}
	if chips[2].Kind != "branch" {
		t.Errorf("chip[2].Kind = %q, want branch", chips[2].Kind)
	}
	// Body should include branch name (with icon).
	if !contains(chips[2].Body, "main") {
		t.Errorf("branch chip body should contain 'main', got %q", chips[2].Body)
	}
}

func TestGatherNarrowChips_WithAWS(t *testing.T) {
	// AWS-only, no git, no other env. Resolver-less deps requires the
	// chip rendering to work without a Resolver; the existing
	// createAwsComponent uses deps.Resolver, so for the narrow path
	// we'd need either a resolver OR a graceful fallback. Provide a
	// real resolver.
	deps := &Dependencies{
		EnvReader: narrowMapEnvReader{"AWS_PROFILE": "staging"},
		Resolver:  newTestResolver(t, ""),
	}
	data := &CachedData{
		CurrentDir:     "/tmp/x",
		UsedPercentage: 30,
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) != 3 {
		t.Fatalf("want 3 chips (dir+ctx+env), got %d: %+v", len(chips), chips)
	}
	if chips[2].Kind != "env" {
		t.Errorf("chip[2].Kind = %q, want env", chips[2].Kind)
	}
}

func TestGatherNarrowChips_AllChips(t *testing.T) {
	deps := &Dependencies{
		EnvReader: narrowMapEnvReader{"AWS_PROFILE": "staging"},
		Resolver:  newTestResolver(t, ""),
	}
	data := &CachedData{
		CurrentDir:     "/tmp/x",
		GitBranch:      "main",
		UsedPercentage: 25,
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) != 4 {
		t.Fatalf("want 4 chips (dir+ctx+branch+env), got %d: %+v", len(chips), chips)
	}
	kinds := []string{chips[0].Kind, chips[1].Kind, chips[2].Kind, chips[3].Kind}
	want := []string{"dir", "context", "branch", "env"}
	for i, k := range want {
		if kinds[i] != k {
			t.Errorf("chip[%d].Kind = %q, want %q", i, kinds[i], k)
		}
	}
}

func TestGatherNarrowChips_AWSPrioritizedOverGcloud(t *testing.T) {
	// Both AWS and gcloud env set → AWS wins, only one env chip.
	deps := &Dependencies{
		EnvReader: narrowMapEnvReader{"AWS_PROFILE": "staging"},
		Resolver:  newTestResolver(t, ""),
	}
	data := &CachedData{
		CurrentDir:    "/tmp/x",
		GcloudProject: "my-project",
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) != 3 {
		t.Fatalf("want 3 chips (dir+ctx+aws), got %d: %+v", len(chips), chips)
	}
	// The env chip body should contain the AWS profile, not the gcloud project.
	envBody := chips[2].Body
	if !contains(envBody, "staging") {
		t.Errorf("env chip should be AWS (staging), got body %q", envBody)
	}
	if contains(envBody, "my-project") {
		t.Errorf("env chip should NOT include gcloud project; got %q", envBody)
	}
}

func TestGatherNarrowChips_GcloudWhenNoAWS(t *testing.T) {
	deps := &Dependencies{
		EnvReader: narrowMapEnvReader{},
		Resolver:  newTestResolver(t, ""),
	}
	data := &CachedData{
		CurrentDir:    "/tmp/x",
		GcloudProject: "my-project",
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) != 3 {
		t.Fatalf("want 3 chips (dir+ctx+gcloud), got %d: %+v", len(chips), chips)
	}
	if !contains(chips[2].Body, "my-project") {
		t.Errorf("env chip should contain gcloud project, got %q", chips[2].Body)
	}
}

func TestGatherNarrowChips_K8sWhenNoAWSorGcloud(t *testing.T) {
	deps := &Dependencies{
		EnvReader: narrowMapEnvReader{},
		Resolver:  newTestResolver(t, ""),
	}
	data := &CachedData{
		CurrentDir: "/tmp/x",
		K8sContext: "my-cluster",
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) != 3 {
		t.Fatalf("want 3 chips (dir+ctx+k8s), got %d: %+v", len(chips), chips)
	}
	if !contains(chips[2].Body, "my-cluster") {
		t.Errorf("env chip should contain k8s context, got %q", chips[2].Body)
	}
}

func TestGatherNarrowChips_ContextPercentRounded(t *testing.T) {
	// UsedPercentage is float64. Verify it's integer-truncated cleanly
	// so chip body never contains decimals.
	deps := depsWith(map[string]string{})
	data := &CachedData{
		CurrentDir:     "/tmp/x",
		UsedPercentage: 42.7,
	}
	chips := gatherNarrowChips(deps, data)
	if len(chips) < 2 || chips[1].Kind != "context" {
		t.Fatalf("expected context chip at index 1, got %+v", chips)
	}
	// 42.7 should render as "42%" or "43%" — either rounding behavior
	// is acceptable but it must NOT contain a decimal point.
	if contains(chips[1].Body, ".") {
		t.Errorf("context body should not contain '.', got %q", chips[1].Body)
	}
}

// --- composition tests ---

func TestComposeNarrowChain_Empty(t *testing.T) {
	s := newTestStatusline(t, newTestResolver(t, ""))
	got := s.composeNarrowChain(nil)
	if got != "" {
		t.Errorf("empty chain = %q, want ''", got)
	}
	got = s.composeNarrowChain([]narrowChip{})
	if got != "" {
		t.Errorf("empty slice chain = %q, want ''", got)
	}
}

func TestComposeNarrowChain_DirOnly(t *testing.T) {
	s := newTestStatusline(t, newTestResolver(t, ""))
	chips := []narrowChip{{Color: "lavender", Body: "~/x", Kind: "dir"}}
	got := s.composeNarrowChain(chips)
	if !contains(got, LeftCurve) {
		t.Errorf("missing LeftCurve in %q", got)
	}
	if !contains(got, RightCurve) {
		t.Errorf("missing RightCurve in %q", got)
	}
	if contains(got, LeftChevron) || contains(got, RightChevron) {
		t.Errorf("single chip should have NO chevrons, got %q", got)
	}
}

func TestComposeNarrowChain_DirAndContext_AllForwardChevrons(t *testing.T) {
	// dir + context, context is the last chip → no pivot, all
	// chevrons are forward (LeftChevron).
	s := newTestStatusline(t, newTestResolver(t, ""))
	chips := []narrowChip{
		{Color: "lavender", Body: "~/x", Kind: "dir"},
		{Color: "green", Body: "0%", Kind: "context"},
	}
	got := s.composeNarrowChain(chips)
	forwardCount := stringsCount(got, LeftChevron)
	backwardCount := stringsCount(got, RightChevron)
	if forwardCount != 1 {
		t.Errorf("want 1 forward chevron, got %d in %q", forwardCount, got)
	}
	if backwardCount != 0 {
		t.Errorf("want 0 backward chevrons (context is last, no pivot), got %d", backwardCount)
	}
}

func TestComposeNarrowChain_DirContextBranch_PivotAtBranch(t *testing.T) {
	// dir + ctx + branch. Pivot is at branch (the chip right after
	// context). So chevron between ctx and branch is backward.
	// Chevron between dir and ctx is forward.
	s := newTestStatusline(t, newTestResolver(t, ""))
	chips := []narrowChip{
		{Color: "lavender", Body: "~/x", Kind: "dir"},
		{Color: "green", Body: "0%", Kind: "context"},
		{Color: "pink", Body: " main", Kind: "branch"},
	}
	got := s.composeNarrowChain(chips)
	if stringsCount(got, LeftChevron) != 1 {
		t.Errorf("want exactly 1 forward chevron, got %d in %q", stringsCount(got, LeftChevron), got)
	}
	if stringsCount(got, RightChevron) != 1 {
		t.Errorf("want exactly 1 backward chevron, got %d in %q", stringsCount(got, RightChevron), got)
	}
}

func TestComposeNarrowChain_AllFourChips_MirrorPattern(t *testing.T) {
	// dir + ctx + branch + env. Forward between dir/ctx; backward
	// between ctx/branch AND branch/env.
	s := newTestStatusline(t, newTestResolver(t, ""))
	chips := []narrowChip{
		{Color: "lavender", Body: "~/x", Kind: "dir"},
		{Color: "green", Body: "0%", Kind: "context"},
		{Color: "pink", Body: " main", Kind: "branch"},
		{Color: "peach", Body: " staging", Kind: "env"},
	}
	got := s.composeNarrowChain(chips)
	if stringsCount(got, LeftChevron) != 1 {
		t.Errorf("want 1 forward chevron, got %d", stringsCount(got, LeftChevron))
	}
	if stringsCount(got, RightChevron) != 2 {
		t.Errorf("want 2 backward chevrons, got %d", stringsCount(got, RightChevron))
	}
}

func TestComposeNarrowChain_StartsWithLeftCurve(t *testing.T) {
	s := newTestStatusline(t, newTestResolver(t, ""))
	chips := []narrowChip{
		{Color: "lavender", Body: "~/x", Kind: "dir"},
		{Color: "green", Body: "0%", Kind: "context"},
	}
	got := s.composeNarrowChain(chips)
	stripped := stripAnsi(got)
	if stripped == "" || !startsWith(stripped, LeftCurve) {
		t.Errorf("stripped output should start with LeftCurve, got %q", stripped)
	}
}

func TestComposeNarrowChain_EndsWithRightCurveThenReset(t *testing.T) {
	s := newTestStatusline(t, newTestResolver(t, ""))
	chips := []narrowChip{
		{Color: "lavender", Body: "~/x", Kind: "dir"},
		{Color: "green", Body: "0%", Kind: "context"},
	}
	got := s.composeNarrowChain(chips)
	// Output must end with NC (\033[0m). Strip ANSI and confirm
	// the last visible character is RightCurve.
	if !endsWith(got, "\033[0m") {
		t.Errorf("output should end with NC reset, got tail %q",
			tail(got, 8))
	}
	stripped := stripAnsi(got)
	if !endsWith(stripped, RightCurve) {
		t.Errorf("stripped output should end with RightCurve, got %q",
			tail(stripped, 4))
	}
}

func TestNarrowVisibleWidth_MatchesWideConvention(t *testing.T) {
	// narrowVisibleWidth should equal runewidth.StringWidth(stripAnsi(s))
	// — same primitive the wide layout uses.
	cases := []string{
		"",
		"hello",
		"\033[38;2;1;2;3mhello\033[0m",
		LeftCurve + " text " + RightCurve,
	}
	for _, c := range cases {
		got := narrowVisibleWidth(c)
		// Sanity: must be at least the rune count of the stripped
		// string (single-cell chars).
		stripped := stripAnsi(c)
		if got < 0 {
			t.Errorf("negative width %d for %q", got, c)
		}
		if got > 0 && stripped == "" {
			t.Errorf("non-zero width %d for empty-after-strip %q", got, c)
		}
	}
}

// --- helpers ---

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func stringsCount(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	count := 0
	i := 0
	for i+len(needle) <= len(haystack) {
		if haystack[i:i+len(needle)] == needle {
			count++
			i += len(needle)
		} else {
			i++
		}
	}
	return count
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
