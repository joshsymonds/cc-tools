package subagentstatusline

import (
	"strings"
	"unicode/utf8"

	"github.com/Veraticus/cc-tools/internal/statusline"
)

// widthMargin reserves room for Claude's row chrome that lives outside
// the subagentStatusLine output. Empirically measured against Claude
// 2.1.145 — the row reserves: 2 cells for the selection indicator
// glyph + space, and 2 cells of left/right padding around the
// decoration. Total budget = 4 cells. If a future Claude version
// changes its row chrome the chain will start wrapping; that's the
// signal to retune this constant.
const widthMargin = 4

// DefaultContextWindow matches the user's Opus 1M default. Exported so
// the cmd/ wrapper consumes one declaration instead of duplicating.
const DefaultContextWindow = 1_000_000

// chainBuilderSize is the initial strings.Builder capacity for one
// chain. Picked so common chains (4-5 chips with ANSI escapes) need at
// most one Grow. Each chip body is ~30 chars + ~60 chars of ANSI per
// transition; 256 covers ~3-4 chips comfortably.
const chainBuilderSize = 256

// visibleWidth returns the displayed cell count of s, ignoring ANSI
// CSI sequences. Single-pass, allocation-free: walks runes, skipping
// any sequence that starts with ESC '[' until the terminating byte
// (a letter A-Za-z) closes the CSI. Powerline glyphs and block chars
// are single-cell in NerdFonts, so rune count after CSI stripping
// equals display width for our vocabulary.
func visibleWidth(s string) int {
	const esc = 0x1b
	count := 0
	i := 0
	for i < len(s) {
		c := s[i]
		if c == esc && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			// Consume until a final byte in [A-Za-z] closes the CSI.
			for i < len(s) {
				b := s[i]
				i++
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					break
				}
			}
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		count++
		i += size
	}
	return count
}

// assembleChain renders chips as an ANSI string framed by LeftCurve
// at the start and RightCurve at the end, with LeftChevron between
// adjacent chips. Color transitions follow the powerline convention:
// each interior chevron has bg=next-chip-color, fg=prev-chip-color so
// the glyph appears to "spill" the previous color into the next chip.
//
// A reset (NC) is emitted between chip body and the following chevron
// for parity with internal/statusline/render.go's renderComponents.
// Equivalent visible output today; defends against future ANSI-state
// leakage if a chip body ever embeds its own escapes.
//
// Returns "" if chips is empty.
func assembleChain(chips []Chip) string {
	if len(chips) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(chainBuilderSize)

	// Leading curve: terminal-default bg, first chip's color in fg.
	b.WriteString(chips[0].Color.FG())
	b.WriteString(statusline.LeftCurve)

	for i, chip := range chips {
		// Chip body: chip's bg + dark base fg + padded body + reset.
		b.WriteString(chip.Color.BG())
		b.WriteString(palette.BaseFG())
		b.WriteString(chip.Body)
		b.WriteString(palette.NC())

		// Separator to the next chip, if any.
		if i+1 < len(chips) {
			next := chips[i+1]
			b.WriteString(next.Color.BG())
			b.WriteString(chip.Color.FG())
			b.WriteString(statusline.LeftChevron)
			b.WriteString(palette.NC())
		}
	}

	// Trailing curve: last chip's color in fg on terminal default bg.
	b.WriteString(chips[len(chips)-1].Color.FG())
	b.WriteString(statusline.RightCurve)
	b.WriteString(palette.NC())

	return b.String()
}

// BuildContent assembles the full chip chain for one task as an ANSI
// string, applying width-pressure drops to fit within the columns
// budget. Chip selection runs in epic-defined order; drops happen
// right-to-left (k8s first, dir never).
//
// contextWindow defaults to DefaultContextWindow (Opus 1M) when 0 or
// negative. snap holds the env values shared across all tasks in this
// invocation — read once per invocation, not per task.
func BuildContent(task Task, columns, contextWindow int, snap EnvSnapshot) string {
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindow
	}

	// Gather all candidate chips in display order. Each entry is
	// (chip, isPresent). Branch and the three env chips are optional.
	dir := renderDirectoryChip(task.CWD)
	ctx := renderContextChip(task.TokenCount, contextWindow)
	branch, branchOK := renderBranchChip(task.CWD)
	aws, awsOK := renderAWSChip(snap.AWSProfile)
	gcloud, gcloudOK := renderGCloudChip(snap.GCloudProject)
	k8s, k8sOK := renderK8sChip(snap.K8sContext)

	// Assemble candidate chip list in display order.
	type opt struct {
		chip    Chip
		present bool
	}
	candidates := []opt{
		{dir, true},        // never dropped
		{ctx, true},        // always rendered (we have tokenCount)
		{branch, branchOK}, // only if .git/HEAD readable
		{aws, awsOK},
		{gcloud, gcloudOK},
		{k8s, k8sOK},
	}

	// Filter out the absent ones first.
	present := make([]Chip, 0, len(candidates))
	for _, c := range candidates {
		if c.present {
			present = append(present, c.chip)
		}
	}

	// Width-pressure: drop right-to-left until the chain fits.
	// Stop at length 1 (directory always remains).
	for len(present) > 1 {
		if visibleWidth(assembleChain(present)) <= columns-widthMargin {
			break
		}
		present = present[:len(present)-1]
	}

	return assembleChain(present)
}
