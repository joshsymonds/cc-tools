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

// --- helpers ---

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
