package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Veraticus/cc-tools/internal/aliases"
	"github.com/mattn/go-runewidth"
)

// narrowWidthThreshold is the upper bound for narrow-mode activation.
// At or below this detected terminal width, Statusline.Render
// dispatches to the narrow renderer; above it, the existing wide
// layout in render.go runs. 80 cells is the natural breakpoint:
// iPhone landscape sits around 80, portrait often well below it,
// and desktop terminals are routinely 100+.
const narrowWidthThreshold = 80

// Context threshold percentages — same as the wide layout for
// consistency. <40 green, <60 yellow, <80 peach, else red.
const (
	contextThresholdYellow = 40
	contextThresholdPeach  = 60
	contextThresholdRed    = 80
)

// Palette key constants. Duplicated as string literals across this
// file and others (env_color.go, render.go right-section); centralizing
// them here keeps `goconst` quiet and makes a future palette refactor
// easier.
const (
	paletteGreen    = "green"
	paletteYellow   = "yellow"
	palettePeach    = "peach"
	paletteRed      = "red"
	paletteLavender = "lavender"
	palettePink     = "pink"
	paletteTeal     = "teal"
)

// Chip kind constants. Stored on narrowChip.Kind to route per-chip
// behavior (truncation order, pivot detection).
const (
	kindDir     = "dir"
	kindContext = "context"
	kindBranch  = "branch"
	kindEnv     = "env"
)

// narrowChip is one chip in the narrow chain.
//
// Color is the palette key (lowercase, matching the strings already
// used in render_clouds.go / env_color.go — "lavender", "pink",
// "peach", "teal", "green", "yellow", "red", etc.). The actual ANSI
// escapes are computed in the composition pass from this key.
//
// Body is the raw printable content (icon + text). The composition
// pass adds the leading/trailing space padding plus chevrons /
// curves. No ANSI escapes embedded.
//
// Kind is "dir", "context", "branch", or "env" — used by the
// truncation pass to know which chips drop first under width
// pressure and which never drop.
type narrowChip struct {
	Color string
	Body  string
	Kind  string
}

// gatherNarrowChips returns the chip list for narrow mode in display
// order: dir, context, optional branch, optional env. The context
// chip is always present (UsedPercentage is always available, even
// if 0). Branch is included when CachedData.GitBranch is non-empty
// (the upstream cache layer already resolves .git/HEAD for us, so
// the renderer doesn't repeat that I/O).
//
// Env chip is a SINGLE chip from one of AWS / gcloud / k8s, in that
// priority order, first non-empty wins.
//
// This pass produces the data shape only — no ANSI, no width
// budgeting, no truncation. Those are handled by later passes.
func gatherNarrowChips(deps *Dependencies, data *CachedData) []narrowChip {
	chips := []narrowChip{
		{Color: paletteLavender, Body: formatPath(data.CurrentDir), Kind: kindDir},
	}

	pct := int(data.UsedPercentage)
	chips = append(chips, narrowChip{
		Color: contextColor(pct),
		Body:  fmt.Sprintf("%d%%", pct),
		Kind:  kindContext,
	})

	if data.GitBranch != "" {
		chips = append(chips, narrowChip{
			Color: palettePink,
			Body:  GitIcon + data.GitBranch,
			Kind:  kindBranch,
		})
	}

	if chip, ok := firstEnvChip(deps, data); ok {
		chips = append(chips, chip)
	}

	return chips
}

// contextColor maps a 0-100 percent to the palette key matching the
// wide layout's thresholds. Green <40, Yellow <60, Peach <80, Red ≥80.
// Identical thresholds to internal/subagentstatusline so the two
// statuslines stay visually consistent.
func contextColor(pct int) string {
	switch {
	case pct < contextThresholdYellow:
		return paletteGreen
	case pct < contextThresholdPeach:
		return paletteYellow
	case pct < contextThresholdRed:
		return palettePeach
	default:
		return paletteRed
	}
}

// firstEnvChip returns the highest-priority env chip available, or
// (narrowChip{}, false) if none are set. Priority is AWS, then
// gcloud project (from CachedData.GcloudProject), then k8s context
// (from CachedData.K8sContext). AWS reads through the existing
// awsProfileFromEnv helper (which strips the literal
// "export AWS_PROFILE=" misconfig pattern that bites users).
//
// Color classification reuses the existing env_color.go helpers via
// the deps.Resolver (env enum lookup → palette key). When no
// Resolver is configured (tests that don't need env-color
// classification), defaults are returned that still render
// meaningfully.
func firstEnvChip(deps *Dependencies, data *CachedData) (narrowChip, bool) {
	if deps.EnvReader != nil {
		if profile := awsProfileFromEnv(deps.EnvReader); profile != "" {
			color := palettePeach // default when no resolver
			if deps.Resolver != nil {
				_, env := deps.Resolver.Resolve(aliases.KindAWS, profile)
				color = awsBgColor(env)
			}
			return narrowChip{
				Color: color,
				Body:  AwsIcon + profile,
				Kind:  kindEnv,
			}, true
		}
	}
	if data.GcloudProject != "" {
		color := paletteLavender
		if deps.Resolver != nil {
			_, env := deps.Resolver.Resolve(aliases.KindGcloud, data.GcloudProject)
			color = gcloudBgColor(env)
		}
		return narrowChip{
			Color: color,
			Body:  GcloudIcon + data.GcloudProject,
			Kind:  kindEnv,
		}, true
	}
	if data.K8sContext != "" {
		color := paletteTeal
		if deps.Resolver != nil {
			_, env := deps.Resolver.Resolve(aliases.KindK8s, data.K8sContext)
			color = k8sBgColor(env)
		}
		return narrowChip{
			Color: color,
			Body:  K8sIcon + data.K8sContext,
			Kind:  kindEnv,
		}, true
	}
	return narrowChip{}, false
}

// composeNarrowChain renders the chip slice as an ANSI string framed
// by LeftCurve at the start and RightCurve at the end. Interior
// chevrons mirror around the context chip: forward-pointing
// (LeftChevron, U+E0B0) before the pivot, backward-pointing
// (RightChevron, U+E0B2) at and after the pivot.
//
// The pivot is defined as "the chip immediately after the context
// chip" — that's where direction reverses. If context is the last
// chip (no chip after it), there's no pivot and all chevrons are
// forward.
//
// Each chip body is wrapped as `<bg><baseFG> <body> <NC>`. Empty
// input returns "". No width-budget logic here — that's the next
// task's job.
func (s *Statusline) composeNarrowChain(chips []narrowChip) string {
	if len(chips) == 0 {
		return ""
	}
	// Initialize colors if the caller didn't already (production
	// path goes through Statusline.Render which sets s.colors; tests
	// constructing chains directly need this guard).
	emptyMocha := CatppuccinMocha{}
	if s.colors == emptyMocha {
		s.colors = CatppuccinMocha{}
	}

	// Pivot index: first chip whose Kind comes AFTER context in
	// display order. We scan for context and take its successor.
	// If context is absent (defensive) or is the last chip, pivot
	// is len(chips) — all chevrons are forward.
	pivot := len(chips)
	for i, c := range chips {
		if c.Kind == kindContext && i+1 < len(chips) {
			pivot = i + 1
			break
		}
	}

	var sb strings.Builder

	// Leading LeftCurve in the first chip's FG, terminal-default bg.
	// chips[0] is safe — guarded by `len(chips) == 0` early-return.
	//nolint:gosec // G602 false positive: length-checked above
	first := chips[0]
	sb.WriteString(s.getColorFG(first.Color))
	sb.WriteString(LeftCurve)
	sb.WriteString(s.colors.NC())

	for i, chip := range chips {
		// Chip body: bg + base fg + padded body + reset.
		sb.WriteString(s.getColorBG(chip.Color))
		sb.WriteString(s.colors.BaseFG())
		sb.WriteString(" ")
		sb.WriteString(chip.Body)
		sb.WriteString(" ")
		sb.WriteString(s.colors.NC())

		// Interior chevron between chip[i] and chip[i+1].
		if i+1 < len(chips) {
			next := chips[i+1]
			sb.WriteString(s.getColorBG(next.Color))
			sb.WriteString(s.getColorFG(chip.Color))
			if i+1 < pivot {
				sb.WriteString(LeftChevron)
			} else {
				sb.WriteString(RightChevron)
			}
			sb.WriteString(s.colors.NC())
		}
	}

	// Trailing RightCurve in the last chip's FG, terminal-default bg.
	last := chips[len(chips)-1]
	sb.WriteString(s.getColorFG(last.Color))
	sb.WriteString(RightCurve)
	sb.WriteString(s.colors.NC())

	return sb.String()
}

// narrowVisibleWidth returns the displayed cell count of s, ignoring
// ANSI escape sequences. Uses the same primitive as the wide layout
// (`runewidth.StringWidth(stripAnsi(s))`) so widths measured here are
// directly comparable across renderers.
func narrowVisibleWidth(s string) int {
	return runewidth.StringWidth(stripAnsi(s))
}

// Width-fit overhead constants. A chain of N chips has:
//   - 1 cell LeftCurve
//   - 1 cell RightCurve
//   - N-1 cells of chevrons (one between each adjacent pair)
//   - 2 cells of padding per chip (the leading/trailing spaces
//     composeNarrowChain adds around each body)
//
// So fixed overhead per chain = 2 + (N-1) + 2N = 3N + 1.
const narrowFixedOverheadPerChip = 3 // 2 padding + 1 chevron (the chevron count is N-1, off-by-one absorbed into +1 below)
const narrowFixedOverheadConst = 1   // 2 (curves) + -1 (chevron count is N-1)

// fitNarrowChain returns the chip slice trimmed and modified so that
// `composeNarrowChain(result)` produces a string of exactly `budget`
// visible cells. The directory chip always survives.
//
// When the chips fit within budget, the context chip's Body is
// expanded (center-aligned content + colored padding) to absorb the
// slack. When chips overflow, they're dropped in priority order:
// env → branch → truncate-dir-to-leaf → drop-context → truncate-dir
// with ellipsis.
//
// Pure function: returns a new slice; input is not mutated.
func fitNarrowChain(chips []narrowChip, budget int) []narrowChip {
	// Copy to avoid mutating caller's slice.
	work := append([]narrowChip(nil), chips...)
	for {
		if len(work) == 0 {
			return work
		}
		n := len(work)
		fixedOverhead := narrowFixedOverheadPerChip*n + narrowFixedOverheadConst
		bodiesSum := 0
		for _, c := range work {
			bodiesSum += runewidth.StringWidth(c.Body)
		}
		total := fixedOverhead + bodiesSum

		switch {
		case total == budget:
			// Exact fit — no expansion or truncation needed.
			return work
		case total < budget:
			// Fits with slack — expand the context chip.
			slack := budget - total
			for i, c := range work {
				if c.Kind == kindContext {
					target := runewidth.StringWidth(c.Body) + slack
					work[i] = narrowChip{
						Color: c.Color,
						Body:  padContextBody(c.Body, target),
						Kind:  c.Kind,
					}
					return work
				}
			}
			// No context chip — give the slack to dir as trailing padding.
			work[0] = narrowChip{
				Color: work[0].Color,
				Body:  work[0].Body + strings.Repeat(" ", slack),
				Kind:  work[0].Kind,
			}
			return work
		}

		// total > budget. Drop one chip / truncate one chip and retry.
		if dropped, ok := dropOneNarrowChip(work); ok {
			work = dropped
			continue
		}
		// No more chips to drop. Last resort: truncate dir to fit.
		// Body should occupy `budget - (3*1 + 1) = budget - 4` cells.
		const dirAloneOverhead = 4
		target := max(1, budget-dirAloneOverhead)
		work[0] = narrowChip{
			Color: work[0].Color,
			Body:  truncateText(work[0].Body, target),
			Kind:  kindDir,
		}
		return work
	}
}

// dropOneNarrowChip drops a single chip per the priority order:
// env first, then branch, then truncate-dir-to-leaf (if dir still
// has path separators), then drop-context. Returns (modifiedSlice,
// true) when a drop/truncate happened, or (slice, false) when there's
// nothing left to drop (only dir remains).
func dropOneNarrowChip(chips []narrowChip) ([]narrowChip, bool) {
	if idx := indexOfKind(chips, kindEnv); idx >= 0 {
		return removeAt(chips, idx), true
	}
	if idx := indexOfKind(chips, kindBranch); idx >= 0 {
		return removeAt(chips, idx), true
	}
	// Try truncating dir to its leaf component.
	if idx := indexOfKind(chips, kindDir); idx >= 0 && strings.Contains(chips[idx].Body, "/") {
		leaf := filepath.Base(chips[idx].Body)
		out := append([]narrowChip(nil), chips...)
		out[idx] = narrowChip{
			Color: chips[idx].Color,
			Body:  leaf,
			Kind:  kindDir,
		}
		return out, true
	}
	// Drop context.
	if idx := indexOfKind(chips, kindContext); idx >= 0 {
		return removeAt(chips, idx), true
	}
	return chips, false
}

func indexOfKind(chips []narrowChip, kind string) int {
	for i, c := range chips {
		if c.Kind == kind {
			return i
		}
	}
	return -1
}

func removeAt(chips []narrowChip, idx int) []narrowChip {
	out := make([]narrowChip, 0, len(chips)-1)
	out = append(out, chips[:idx]...)
	out = append(out, chips[idx+1:]...)
	return out
}

// padContextBody centers the original body inside a wider chip body
// of `targetWidth` cells. Odd slack puts the extra cell on the right
// (by convention). If targetWidth ≤ width(body), returns body
// unchanged — let the chain be slightly over-budget rather than
// corrupt the content.
func padContextBody(body string, targetWidth int) string {
	current := runewidth.StringWidth(body)
	if targetWidth <= current {
		return body
	}
	const halfDivisor = 2
	slack := targetWidth - current
	left := slack / halfDivisor
	right := slack - left
	return strings.Repeat(" ", left) + body + strings.Repeat(" ", right)
}

// renderNarrow is the top-level narrow-mode entry point. Gathers
// chips, fits to width, composes the ANSI chain. The result's
// visible width is exactly `width` cells under normal conditions —
// any deviation is logged when DEBUG_WIDTH=1 so smoke tests catch
// math regressions.
func (s *Statusline) renderNarrow(data *CachedData, width int) string {
	chips := gatherNarrowChips(s.deps, data)
	fitted := fitNarrowChain(chips, width)
	result := s.composeNarrowChain(fitted)
	if os.Getenv("DEBUG_WIDTH") == "1" {
		actual := narrowVisibleWidth(result)
		if actual != width {
			fmt.Fprintf(os.Stderr,
				"renderNarrow: budget=%d actual=%d chips=%d\n",
				width, actual, len(fitted))
		}
	}
	return result
}
