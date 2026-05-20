package subagentstatusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Veraticus/cc-tools/internal/statusline"
)

// EnvReader is the small interface every env-driven chip uses. The
// production wiring plugs in statusline.DefaultEnvReader{} which
// already overlays the cc-tools state file on top of process env for
// AWS_PROFILE — meaning a Bash-tool export in a subagent reaches the
// agent-view's chip via that file rather than via process inheritance.
type EnvReader interface {
	Get(key string) string
}

// Color is a palette entry name. The methods FG()/BG() delegate to
// the existing CatppuccinMocha palette in internal/statusline/colors.go
// so the hex values aren't duplicated here.
//
// Stored as a string so chips and decorations are cheap value types
// the chain builder can pass around freely.
type Color string

// Palette entries this package uses. The set is intentionally narrow:
// adding a new chip color is a deliberate decision, not an accident.
const (
	ColorLavender Color = "lavender"
	ColorGreen    Color = "green"
	ColorYellow   Color = "yellow"
	ColorPeach    Color = "peach"
	ColorRed      Color = "red"
	ColorPink     Color = "pink"
	ColorTeal     Color = "teal"
)

// chip text uses the dark base foreground so the colored bg stays
// readable; consistent with how internal/statusline renders chips.
var palette = statusline.CatppuccinMocha{} //nolint:gochecknoglobals // palette is a stateless value, shared

// BG returns the 24-bit ANSI background escape for this color.
// Returns "" for unknown colors so chain assembly degrades visibly
// rather than panicking; new colors must be added explicitly here.
func (c Color) BG() string {
	switch c {
	case ColorLavender:
		return palette.LavenderBG()
	case ColorGreen:
		return palette.GreenBG()
	case ColorYellow:
		return palette.YellowBG()
	case ColorPeach:
		return palette.PeachBG()
	case ColorRed:
		return palette.RedBG()
	case ColorPink:
		return palette.PinkBG()
	case ColorTeal:
		return palette.TealBG()
	}
	return ""
}

// FG returns the 24-bit ANSI foreground escape for this color. Used
// for chevron transitions (chevron's fg = previous chip's color) and
// for the LeftCurve/RightCurve end-caps (fg of the touching chip).
func (c Color) FG() string {
	switch c {
	case ColorLavender:
		return palette.LavenderFG()
	case ColorGreen:
		return palette.GreenFG()
	case ColorYellow:
		return palette.YellowFG()
	case ColorPeach:
		return palette.PeachFG()
	case ColorRed:
		return palette.RedFG()
	case ColorPink:
		return palette.PinkFG()
	case ColorTeal:
		return palette.TealFG()
	}
	return ""
}

// Chip is one rendered powerline cell with Body padded and ready to
// be wrapped by the chain builder. Body never contains chevrons or
// end-caps — those are the chain builder's responsibility.
type Chip struct {
	// Color is the background palette entry. Chain assembly uses
	// Color.FG() to compute chevron transitions to adjacent chips.
	Color Color

	// Body is the printable content with a leading and trailing space
	// so it seats cleanly inside curves/chevrons (which butt directly
	// against the chip background).
	Body string
}

// renderDirectoryChip builds the directory chip. The cwd is shortened
// to ~/<rel> when under $HOME, kept as-is otherwise. Empty cwd
// defensively renders "?" rather than panicking — the directory chip
// always appears in the chain.
func renderDirectoryChip(cwd string) Chip {
	return Chip{
		Color: ColorLavender,
		Body:  " " + shortenCWD(cwd) + " ",
	}
}

func shortenCWD(cwd string) string {
	if cwd == "" {
		return "?"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cwd
	}
	if cwd == home {
		return "~"
	}
	if strings.HasPrefix(cwd, home+"/") {
		return "~" + cwd[len(home):]
	}
	return cwd
}

// Context-bar tuning constants. Keeping them named so reading the
// renderContextChip body doesn't require chasing magic numbers.
const (
	contextThresholdYellow = 40
	contextThresholdPeach  = 60
	contextThresholdRed    = 80
	contextQuintileSize    = 20 // 100% / 5 quintiles
	contextMaxQuintiles    = 5
	contextPercentFull     = 100
)

// renderContextChip builds the context bar chip. Format is exactly
// " ▰▰▰▱▱ NN% " — leading space, 5 quintile blocks (filled by usage),
// space, percent, trailing space. Background shifts Green / Yellow /
// Peach / Red at 40 / 60 / 80% thresholds.
//
// Defensive against bad input: negative tokens clamp to 0; tokens
// exceeding contextWindow clamp to 100%.
func renderContextChip(tokenCount, contextWindow int) Chip {
	if contextWindow <= 0 {
		contextWindow = 1
	}
	pct := contextPercentFull * tokenCount / contextWindow
	pct = max(0, pct)
	pct = min(contextPercentFull, pct)

	filled := min(contextMaxQuintiles, pct/contextQuintileSize)
	empty := contextMaxQuintiles - filled

	body := " " + strings.Repeat("▰", filled) + strings.Repeat("▱", empty) +
		fmt.Sprintf(" %d%% ", pct)

	var color Color
	switch {
	case pct < contextThresholdYellow:
		color = ColorGreen
	case pct < contextThresholdPeach:
		color = ColorYellow
	case pct < contextThresholdRed:
		color = ColorPeach
	default:
		color = ColorRed
	}
	return Chip{Color: color, Body: body}
}

// shortSHALen is how many hex chars of a commit SHA we display for
// detached HEADs. 8 is git's default abbreviation length.
const shortSHALen = 8

// renderBranchChip reads <cwd>/.git/HEAD and returns the branch chip.
// Returns (Chip{}, false) when there is no readable .git directory
// (worktree-pointer files where .git is a regular file are tolerated
// as "no chip" — parsing those pointers is intentionally out of scope).
//
// No subprocess fork. Single os.ReadFile syscall per task.
func renderBranchChip(cwd string) (Chip, bool) {
	if cwd == "" {
		return Chip{}, false
	}
	gitDir := filepath.Join(cwd, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return Chip{}, false
	}
	headPath := filepath.Join(gitDir, "HEAD")
	//nolint:gosec // cwd-relative HEAD; daemon-side this is the agent's own cwd from the JSON payload
	raw, err := os.ReadFile(headPath)
	if err != nil {
		return Chip{}, false
	}
	head := strings.TrimSpace(string(raw))
	var name string
	switch {
	case strings.HasPrefix(head, "ref: refs/heads/"):
		name = strings.TrimPrefix(head, "ref: refs/heads/")
	case len(head) >= shortSHALen:
		name = head[:shortSHALen]
	default:
		return Chip{}, false
	}
	if name == "" {
		return Chip{}, false
	}
	return Chip{
		Color: ColorPink,
		Body:  " " + statusline.GitIcon + name + " ",
	}, true
}

// renderAWSChip returns the AWS chip from env. Profile names
// containing "prod" (case-insensitive) get Peach; everything else
// gets Teal. Returns (Chip{}, false) when AWS_PROFILE is unset/empty.
func renderAWSChip(env EnvReader) (Chip, bool) {
	profile := env.Get("AWS_PROFILE")
	if profile == "" {
		return Chip{}, false
	}
	color := ColorTeal
	if strings.Contains(strings.ToLower(profile), "prod") {
		color = ColorPeach
	}
	return Chip{
		Color: color,
		Body:  " " + statusline.AwsIcon + profile + " ",
	}, true
}

// renderGCloudChip returns the gcloud chip from env. CLOUDSDK_CORE_PROJECT
// takes precedence over GOOGLE_CLOUD_PROJECT — gcloud itself reads the
// former when both are set. Returns (Chip{}, false) when neither is set.
func renderGCloudChip(env EnvReader) (Chip, bool) {
	project := env.Get("CLOUDSDK_CORE_PROJECT")
	if project == "" {
		project = env.Get("GOOGLE_CLOUD_PROJECT")
	}
	if project == "" {
		return Chip{}, false
	}
	return Chip{
		Color: ColorPeach,
		Body:  " " + statusline.GcloudIcon + project + " ",
	}, true
}

// renderK8sChip returns the kubernetes context chip from env. Reads
// KUBE_CONTEXT first, then KUBERNETES_CONTEXT. Returns (Chip{}, false)
// when neither is set.
//
// Note: the true source of "current kubectl context" is ~/.kube/config
// under current-context, but parsing YAML adds a dependency we don't
// otherwise need. If env-driven sourcing proves insufficient in
// practice, that's the right next step.
func renderK8sChip(env EnvReader) (Chip, bool) {
	ctx := env.Get("KUBE_CONTEXT")
	if ctx == "" {
		ctx = env.Get("KUBERNETES_CONTEXT")
	}
	if ctx == "" {
		return Chip{}, false
	}
	return Chip{
		Color: ColorTeal,
		Body:  " " + statusline.K8sIcon + ctx + " ",
	}, true
}
