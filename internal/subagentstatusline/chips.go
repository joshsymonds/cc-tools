package subagentstatusline

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// EnvSnapshot captures the env values the env-driven chips need at
// a single point in time. SnapshotEnv reads them once per invocation
// so a multi-task batch doesn't repeatedly re-open the AWS_PROFILE
// state file (which DefaultEnvReader.Get does on every call).
type EnvSnapshot struct {
	AWSProfile    string
	GCloudProject string
	K8sContext    string
}

// SnapshotEnv reads the relevant env values once, applying the same
// precedence used by the individual chip helpers (CLOUDSDK over GOOGLE,
// KUBE_CONTEXT over KUBERNETES_CONTEXT). Callers that need to do
// per-task rendering should snapshot once and pass the result down,
// not call this per task.
func SnapshotEnv(env EnvReader) EnvSnapshot {
	snap := EnvSnapshot{
		AWSProfile:    env.Get("AWS_PROFILE"),
		GCloudProject: env.Get("CLOUDSDK_CORE_PROJECT"),
		K8sContext:    env.Get("KUBE_CONTEXT"),
	}
	if snap.GCloudProject == "" {
		snap.GCloudProject = env.Get("GOOGLE_CLOUD_PROJECT")
	}
	if snap.K8sContext == "" {
		snap.K8sContext = env.Get("KUBERNETES_CONTEXT")
	}
	return snap
}

// validBranchName rejects branch names that would inject ANSI escapes
// or otherwise render badly: any C0/C1 control bytes (0x00-0x1f, 0x7f),
// spaces, or a leading dash (git's own ref-naming rule). Compiled
// once at package init.
var validBranchName = regexp.MustCompile(`^[^\x00-\x20\x7f][^\x00-\x1f\x7f ]*$`)

// stripControl strips C0/C1 control bytes from a string. Used on
// untrusted env values before they enter chip bodies — otherwise a
// poisoned AWS profile name (via the state file) or gcloud/k8s env
// could inject terminal escape sequences into the rendered tty.
func stripControl(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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
// to ~/<rel> when under $HOME, kept as-is otherwise. Empty cwd falls
// back to the process's current working directory; if that's also
// unavailable, "?" is the final fallback. The directory chip is
// never dropped from the chain, so a placeholder is always rendered.
func renderDirectoryChip(cwd string) Chip {
	return Chip{
		Color: ColorLavender,
		Body:  " " + stripControl(shortenCWD(cwd)) + " ",
	}
}

func shortenCWD(cwd string) string {
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil && wd != "" {
			cwd = wd
		} else {
			return "?"
		}
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

	// Quintile count uses ROUND (half-up), not floor. Per epic spec
	// `min(5, round(tokens/window * 5))`. Implemented in integers as
	// `(roundDoubler*pct + quintileSize) / (roundDoubler*quintileSize)` —
	// equivalent to `round(pct/20)` without pulling in math.Round.
	const roundDoubler = 2
	filled := min(contextMaxQuintiles,
		(roundDoubler*pct+contextQuintileSize)/(roundDoubler*contextQuintileSize))
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

// renderBranchChip reads <cwd>/.git/HEAD (or dereferences a worktree
// gitdir pointer) and returns the branch chip. Returns (Chip{}, false)
// when:
//   - cwd is empty
//   - no .git exists
//   - .git is a symlink (rejected as a defense against arbitrary-file
//     reads via planted symlinks; same for HEAD)
//   - HEAD content doesn't yield a recognizable branch name
//   - branch name fails validBranchName (rejects names with control
//     bytes, spaces, or leading dash — git's own rules + ANSI safety)
//
// No subprocess fork. At most three syscalls (stat .git, read .git or
// HEAD, optional read of worktree HEAD).
func renderBranchChip(cwd string) (Chip, bool) {
	if cwd == "" {
		return Chip{}, false
	}
	headPath, ok := resolveGitHEADPath(cwd)
	if !ok {
		return Chip{}, false
	}
	// Lstat HEAD before reading: refuse a symlinked HEAD that crosses
	// the cwd boundary. (Inside the .git dir, HEAD is always a regular
	// file in normal repositories.)
	headInfo, err := os.Lstat(headPath)
	if err != nil || headInfo.Mode()&os.ModeSymlink != 0 {
		return Chip{}, false
	}
	//nolint:gosec // resolved path validated above
	raw, err := os.ReadFile(headPath)
	if err != nil {
		return Chip{}, false
	}
	head := strings.TrimSpace(string(raw))
	var name string
	switch {
	case strings.HasPrefix(head, "ref: refs/heads/"):
		name = strings.TrimPrefix(head, "ref: refs/heads/")
	case len(head) >= shortSHALen && isHexPrefix(head, shortSHALen):
		name = head[:shortSHALen]
	default:
		return Chip{}, false
	}
	if !validBranchName.MatchString(name) {
		return Chip{}, false
	}
	return Chip{
		Color: ColorPink,
		Body:  " " + statusline.GitIcon + name + " ",
	}, true
}

// resolveGitHEADPath returns the absolute path to HEAD for the
// repository at cwd, supporting both plain repos (.git is a directory)
// and worktrees (.git is a file with `gitdir: <path>`). Returns false
// if .git is a symlink (refused — same threat model as HEAD symlink),
// missing, or otherwise unresolvable.
func resolveGitHEADPath(cwd string) (string, bool) {
	gitPath := filepath.Join(cwd, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "HEAD"), true
	}
	// Worktree pointer: a regular file containing `gitdir: <real-path>`.
	//nolint:gosec // gitPath inside a cwd we already trust
	raw, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		if rest, ok := strings.CutPrefix(line, "gitdir: "); ok {
			gitDir := strings.TrimSpace(rest)
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(cwd, gitDir)
			}
			return filepath.Join(gitDir, "HEAD"), true
		}
	}
	return "", false
}

// isHexPrefix returns true when the first n bytes of s are all hex
// digits. Used to distinguish a real detached-HEAD SHA from a
// symlinked-into-arbitrary-file degenerate case.
func isHexPrefix(s string, n int) bool {
	if len(s) < n {
		return false
	}
	for i := range n {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// renderAWSChip builds the AWS chip from a pre-fetched profile.
// Empty profile → (Chip{}, false). Profile names containing "prod"
// (case-insensitive) get Peach; others Teal.
//
// This is a deliberately simpler classifier than the main statusline's
// awsBgColor (internal/statusline/env_color.go) which resolves through
// an alias table into a four-state Red/Peach/Green/Teal scheme. The
// subagent statusline runs without the alias resolver dependency, so
// a binary substring-of-"prod" → Peach is the budget-tier rule. Two
// statuslines may therefore color the same profile differently — by
// design, not a bug.
func renderAWSChip(profile string) (Chip, bool) {
	clean := stripControl(profile)
	if clean == "" {
		return Chip{}, false
	}
	color := ColorTeal
	if strings.Contains(strings.ToLower(clean), "prod") {
		color = ColorPeach
	}
	return Chip{
		Color: color,
		Body:  " " + statusline.AwsIcon + clean + " ",
	}, true
}

// renderGCloudChip builds the gcloud chip from a pre-fetched project.
// Empty project → (Chip{}, false).
func renderGCloudChip(project string) (Chip, bool) {
	clean := stripControl(project)
	if clean == "" {
		return Chip{}, false
	}
	return Chip{
		Color: ColorPeach,
		Body:  " " + statusline.GcloudIcon + clean + " ",
	}, true
}

// renderK8sChip builds the kubernetes context chip from a pre-fetched
// context. Empty context → (Chip{}, false).
//
// Note: the true source of "current kubectl context" is ~/.kube/config
// under current-context, but parsing YAML adds a dependency we don't
// otherwise need. If env-driven sourcing proves insufficient in
// practice, that's the right next step.
func renderK8sChip(ctx string) (Chip, bool) {
	clean := stripControl(ctx)
	if clean == "" {
		return Chip{}, false
	}
	return Chip{
		Color: ColorTeal,
		Body:  " " + statusline.K8sIcon + clean + " ",
	}, true
}
