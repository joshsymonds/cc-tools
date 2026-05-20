package subagentstatusline

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Veraticus/cc-tools/internal/statusline"
)

// widthMargin reserves room for Claude's row chrome (the agent name
// label, selection indicator, etc.). Empirically ~4 cells is enough;
// under-reserving causes wrap, over-reserving wastes chip space.
const widthMargin = 4

// defaultContextWindow matches the user's Opus 1M default. Callers can
// override via the contextWindow parameter to BuildContent.
const defaultContextWindow = 1_000_000

// csiRegex matches ANSI CSI sequences like `\033[38;2;180;190;254m`.
// Used to strip them when measuring printable width. Compiled once at
// package init.
var csiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// visibleWidth returns the displayed cell count of s, ignoring CSI
// escapes. Powerline glyphs and block characters are single-cell in
// any NerdFont, so rune count after escape stripping equals display
// width for our chip vocabulary.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(csiRegex.ReplaceAllString(s, ""))
}

// assembleChain renders chips as an ANSI string framed by LeftCurve
// at the start and RightCurve at the end, with LeftChevron between
// adjacent chips. Color transitions follow the powerline convention:
// each interior chevron has bg=next-chip-color, fg=prev-chip-color so
// the glyph appears to "spill" the previous color into the next chip.
//
// Returns "" if chips is empty.
func assembleChain(chips []Chip) string {
	if len(chips) == 0 {
		return ""
	}
	var b strings.Builder

	// Leading curve: terminal-default bg, first chip's color in fg.
	b.WriteString(chips[0].Color.FG())
	b.WriteString(statusline.LeftCurve)

	for i, chip := range chips {
		// Chip body: chip's bg + dark base fg + padded body.
		b.WriteString(chip.Color.BG())
		b.WriteString(palette.BaseFG())
		b.WriteString(chip.Body)

		// Separator to the next chip, if any.
		if i+1 < len(chips) {
			next := chips[i+1]
			b.WriteString(next.Color.BG())
			b.WriteString(chip.Color.FG())
			b.WriteString(statusline.LeftChevron)
		}
	}

	// Trailing curve: clear the chip's bg first so the curve sits on
	// the terminal default; last chip's color in fg.
	b.WriteString(palette.NC())
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
// contextWindow defaults to 1_000_000 (Opus 1M) when 0 or negative.
// env supplies AWS_PROFILE, CLOUDSDK_CORE_PROJECT / GOOGLE_CLOUD_PROJECT,
// KUBE_CONTEXT / KUBERNETES_CONTEXT.
func BuildContent(task Task, columns, contextWindow int, env EnvReader) string {
	if contextWindow <= 0 {
		contextWindow = defaultContextWindow
	}

	// Gather all candidate chips in display order. Each entry is
	// (chip, isPresent). Branch and the three env chips are optional.
	dir := renderDirectoryChip(task.CWD)
	ctx := renderContextChip(task.TokenCount, contextWindow)
	branch, branchOK := renderBranchChip(task.CWD)
	aws, awsOK := renderAWSChip(env)
	gcloud, gcloudOK := renderGCloudChip(env)
	k8s, k8sOK := renderK8sChip(env)

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
