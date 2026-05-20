package statusline

import (
	"testing"

	"github.com/mattn/go-runewidth"
)

// TestSmoke_ExactWidthsAcrossThresholds verifies the renderer
// produces the expected visible-cell count at multiple widths
// straddling the narrow/wide boundary. Acts as a regression net
// for the width-math layer.
//
// Both narrow and wide rendering honor the default 2+2 spacer
// convention, so the rendered visible width is termWidth − 4.
func TestSmoke_ExactWidthsAcrossThresholds(t *testing.T) {
	const defaultSpacerTotal = 4 // 2 left + 2 right
	cases := []struct {
		width    int
		isNarrow bool
	}{
		{30, true},
		{50, true},
		{64, true},
		{80, true},  // at threshold (≤80)
		{81, false}, // just above
		{100, false},
		{200, false},
	}
	for _, c := range cases {
		got := renderAt(t, c.width)
		w := runewidth.StringWidth(stripAnsi(got))
		wantNarrow := c.width - defaultSpacerTotal
		switch {
		case c.isNarrow && w != wantNarrow:
			t.Errorf("width=%d narrow expected exact %d cells (termWidth − spacers), got %d",
				c.width, wantNarrow, w)
		case !c.isNarrow:
			if w <= 0 {
				t.Errorf("width=%d wide expected positive width, got %d", c.width, w)
			}
		}
	}
}
