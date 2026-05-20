package subagentstatusline

import (
	"strings"
	"testing"
)

func TestRenderDirectoryChip_EmptyCWD(t *testing.T) {
	got := renderDirectoryChip("")
	if !strings.Contains(got.Body, "?") {
		t.Errorf("empty cwd should render as '?', got Body=%q", got.Body)
	}
	if got.Color != ColorLavender {
		t.Errorf("color = %v, want ColorLavender", got.Color)
	}
}

func TestRenderDirectoryChip_HomeItself(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got := renderDirectoryChip("/tmp/fakehome")
	// "~" exactly — not "~/" (no trailing slash for home itself)
	if !strings.Contains(got.Body, "~") {
		t.Errorf("home cwd should render with ~, got Body=%q", got.Body)
	}
	if strings.Contains(got.Body, "~/") {
		t.Errorf("home cwd should NOT contain ~/ (trailing slash), got Body=%q", got.Body)
	}
}

func TestRenderDirectoryChip_UnderHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got := renderDirectoryChip("/tmp/fakehome/projects/cc-tools")
	if !strings.Contains(got.Body, "~/projects/cc-tools") {
		t.Errorf("under-home cwd should render as ~/relative, got Body=%q", got.Body)
	}
}

func TestRenderDirectoryChip_OutsideHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got := renderDirectoryChip("/etc/nixos")
	if !strings.Contains(got.Body, "/etc/nixos") {
		t.Errorf("outside-home cwd should render absolute path, got Body=%q", got.Body)
	}
	if strings.Contains(got.Body, "~") {
		t.Errorf("outside-home cwd should NOT use ~, got Body=%q", got.Body)
	}
}

func TestRenderDirectoryChip_Root(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got := renderDirectoryChip("/")
	if !strings.Contains(got.Body, "/") {
		t.Errorf("root cwd should render as /, got Body=%q", got.Body)
	}
}

func TestRenderDirectoryChip_PaddingSpaces(t *testing.T) {
	got := renderDirectoryChip("/tmp/x")
	// Body must have leading and trailing space for chevron-chain seating.
	if !strings.HasPrefix(got.Body, " ") {
		t.Errorf("Body should start with a space, got %q", got.Body)
	}
	if !strings.HasSuffix(got.Body, " ") {
		t.Errorf("Body should end with a space, got %q", got.Body)
	}
}

func TestRenderContextChip_Zero(t *testing.T) {
	got := renderContextChip(0, 1_000_000)
	if got.Color != ColorGreen {
		t.Errorf("0%% color = %v, want ColorGreen", got.Color)
	}
	if !strings.Contains(got.Body, "▱▱▱▱▱") {
		t.Errorf("0%% should have 5 empty blocks, got Body=%q", got.Body)
	}
	if strings.Contains(got.Body, "▰") {
		t.Errorf("0%% should have NO filled blocks, got Body=%q", got.Body)
	}
	if !strings.Contains(got.Body, "0%") {
		t.Errorf("Body should contain percent, got %q", got.Body)
	}
}

func TestRenderContextChip_FortyPercent(t *testing.T) {
	got := renderContextChip(400_000, 1_000_000)
	// 40% / 20 = 2 quintile blocks filled
	filled := strings.Count(got.Body, "▰")
	empty := strings.Count(got.Body, "▱")
	if filled != 2 || empty != 3 {
		t.Errorf("40%% blocks = %d filled, %d empty; want 2 filled, 3 empty (Body=%q)", filled, empty, got.Body)
	}
	if got.Color != ColorYellow {
		t.Errorf("40%% color = %v, want ColorYellow", got.Color)
	}
}

func TestRenderContextChip_EightyPercent(t *testing.T) {
	got := renderContextChip(800_000, 1_000_000)
	filled := strings.Count(got.Body, "▰")
	if filled != 4 {
		t.Errorf("80%% filled = %d, want 4 (Body=%q)", filled, got.Body)
	}
	if got.Color != ColorRed {
		t.Errorf("80%% color = %v, want ColorRed", got.Color)
	}
}

func TestRenderContextChip_FullWindow(t *testing.T) {
	got := renderContextChip(1_000_000, 1_000_000)
	filled := strings.Count(got.Body, "▰")
	if filled != 5 {
		t.Errorf("100%% filled = %d, want 5", filled)
	}
	if !strings.Contains(got.Body, "100%") {
		t.Errorf("100%% body should contain '100%%', got %q", got.Body)
	}
	if got.Color != ColorRed {
		t.Errorf("100%% color = %v, want ColorRed", got.Color)
	}
}

func TestRenderContextChip_OverWindow(t *testing.T) {
	got := renderContextChip(1_500_000, 1_000_000)
	filled := strings.Count(got.Body, "▰")
	if filled != 5 {
		t.Errorf(">100%% filled = %d, want 5 (clamped)", filled)
	}
	if !strings.Contains(got.Body, "100%") {
		t.Errorf(">100%% body should clamp display to 100%%, got %q", got.Body)
	}
	if got.Color != ColorRed {
		t.Errorf(">100%% color = %v, want ColorRed", got.Color)
	}
}

func TestRenderContextChip_NegativeTokens(t *testing.T) {
	got := renderContextChip(-100, 1_000_000)
	filled := strings.Count(got.Body, "▰")
	if filled != 0 {
		t.Errorf("negative tokens filled = %d, want 0 (clamped)", filled)
	}
	if got.Color != ColorGreen {
		t.Errorf("negative tokens color = %v, want ColorGreen (clamped to 0%%)", got.Color)
	}
}

func TestRenderContextChip_ThresholdTransitions(t *testing.T) {
	// Verify each boundary transition.
	cases := []struct {
		pct       int
		wantColor Color
	}{
		{0, ColorGreen},
		{39, ColorGreen},
		{40, ColorYellow},
		{59, ColorYellow},
		{60, ColorPeach},
		{79, ColorPeach},
		{80, ColorRed},
		{99, ColorRed},
		{100, ColorRed},
	}
	const window = 1_000_000
	for _, c := range cases {
		tokens := c.pct * window / 100
		got := renderContextChip(tokens, window)
		if got.Color != c.wantColor {
			t.Errorf("%d%% (tokens=%d): color = %v, want %v", c.pct, tokens, got.Color, c.wantColor)
		}
	}
}

func TestRenderContextChip_AlternateWindow(t *testing.T) {
	// 200k context (Sonnet-style) — 100k tokens = 50% = Yellow.
	got := renderContextChip(100_000, 200_000)
	if got.Color != ColorYellow {
		t.Errorf("100k/200k color = %v, want ColorYellow (50%%)", got.Color)
	}
	if !strings.Contains(got.Body, "50%") {
		t.Errorf("Body should contain 50%%, got %q", got.Body)
	}
	// 2 quintile blocks filled (50/20 = 2).
	filled := strings.Count(got.Body, "▰")
	if filled != 2 {
		t.Errorf("50%% filled = %d, want 2", filled)
	}
}

func TestRenderContextChip_ZeroWindow(t *testing.T) {
	// Defensive: contextWindow=0 should not divide-by-zero.
	got := renderContextChip(100, 0)
	// With window clamped to 1, 100/1 = 100% → Red, 5 blocks.
	if got.Color != ColorRed {
		t.Errorf("zero-window color = %v, want ColorRed (clamped)", got.Color)
	}
}

func TestRenderContextChip_PaddingSpaces(t *testing.T) {
	got := renderContextChip(500_000, 1_000_000)
	if !strings.HasPrefix(got.Body, " ") {
		t.Errorf("Body should start with space, got %q", got.Body)
	}
	if !strings.HasSuffix(got.Body, " ") {
		t.Errorf("Body should end with space, got %q", got.Body)
	}
}

func TestColor_FGBG_Roundtrip(t *testing.T) {
	// Sanity: each color exposes non-empty FG and BG escape strings.
	colors := []Color{ColorLavender, ColorGreen, ColorYellow, ColorPeach, ColorRed, ColorPink, ColorTeal}
	for _, c := range colors {
		if c.FG() == "" {
			t.Errorf("%v FG() returned empty", c)
		}
		if c.BG() == "" {
			t.Errorf("%v BG() returned empty", c)
		}
		// FG and BG must differ (different ANSI selectors: 38 vs 48).
		if c.FG() == c.BG() {
			t.Errorf("%v FG and BG should differ", c)
		}
	}
}
