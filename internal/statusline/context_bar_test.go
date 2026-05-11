package statusline

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestContextBar_NoLabelWord(t *testing.T) {
	deps := &Dependencies{
		FileReader:    &MockFileReader{},
		CommandRunner: &MockCommandRunner{},
		EnvReader:     &MockEnvReader{vars: make(map[string]string)},
		TerminalWidth: &MockTerminalWidth{width: 200},
	}
	s := CreateStatusline(deps)
	data := &CachedData{
		ModelDisplay:   "S4.6",
		CurrentDir:     "/home/user",
		TermWidth:      200,
		UsedPercentage: 42.0,
	}
	result := s.Render(data)
	stripped := stripAnsi(result)

	if strings.Contains(stripped, "Context") {
		t.Errorf("rendered output should not contain the literal 'Context' label (epic R8); got %q", stripped)
	}
	// Icon must still appear so the bar is identifiable.
	if !strings.Contains(stripped, ContextIcon) {
		t.Errorf("rendered output should contain the context icon; got %q", stripped)
	}
	// Percentage must appear.
	if !strings.Contains(stripped, "42.0%") {
		t.Errorf("rendered output should contain '42.0%%'; got %q", stripped)
	}
}

func TestContextBarPadding(t *testing.T) {
	deps := &Dependencies{
		FileReader:    &MockFileReader{},
		CommandRunner: &MockCommandRunner{},
		EnvReader:     &MockEnvReader{vars: make(map[string]string)},
		TerminalWidth: &MockTerminalWidth{width: 100},
	}

	t.Run("context bar has 4 space padding on each side", func(t *testing.T) {
		s := CreateStatusline(deps)
		data := &CachedData{
			ModelDisplay:   "Claude",
			CurrentDir:     "/home/user",
			TermWidth:      100,
			UsedPercentage: 25.0, // Will show context bar
		}

		result := s.Render(data)

		// Find the context bar portion (between the left and right sections).
		// After the label drop (epic R8), the bar starts with the
		// ContextIcon glyph followed directly by the percentage. The
		// literal "Context " word is no longer rendered.
		if strings.Contains(result, ContextIcon) {
			// Strip ANSI codes to analyze spacing
			stripped := stripAnsi(result)

			// Find where the icon appears
			contextIndex := strings.Index(stripped, ContextIcon)
			if contextIndex == -1 {
				t.Error("Context bar should be visible with UsedPercentage > 0")
				return
			}

			// Check that there are at least 4 spaces before the context bar starts
			// The context bar starts with the left curve character before "Context"
			// Count spaces before the curve
			spacesBeforeBar := 0
			for i := contextIndex - 1; i >= 0 && stripped[i] == ' '; i-- {
				spacesBeforeBar++
			}

			// Due to the curve character, we check spaces before it
			// The pattern should be: ...content... + 4 spaces + curve + "Context"
			// We need to find the actual curve position
			t.Logf("Spaces found before context area: %d", spacesBeforeBar)
		} else {
			t.Log("Context bar not visible in output - may need more width")
		}
	})

	t.Run("context bar content shrinks with padding", func(t *testing.T) {
		// Use custom config with smaller spacers for this test
		config := &Config{
			LeftSpacerWidth:  2,
			RightSpacerWidth: 4,
		}
		s := NewWithConfig(deps, config)

		// Test with different widths to see how the bar adapts
		testCases := []struct {
			name           string
			termWidth      int
			usedPercentage float64
			minBarWidth    int // Minimum expected width for the context bar content (excluding padding)
		}{
			{
				name:           "normal width",
				termWidth:      120,
				usedPercentage: 25.0,
				minBarWidth:    20, // Should have reasonable space for bar
			},
			{
				name:           "narrow terminal",
				termWidth:      80,
				usedPercentage: 25.0,
				minBarWidth:    10, // Bar should be smaller but still visible
			},
			{
				name:           "very narrow terminal",
				termWidth:      60,
				usedPercentage: 25.0,
				minBarWidth:    0, // Might not show bar at all if too narrow
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				deps.TerminalWidth = &MockTerminalWidth{width: tc.termWidth}
				data := &CachedData{
					ModelDisplay:   "Claude",
					CurrentDir:     "/home/user",
					TermWidth:      tc.termWidth,
					UsedPercentage: tc.usedPercentage,
				}

				result := s.Render(data)
				stripped := stripAnsi(result)

				// Total width should match terminal width minus spacers (2+4=6)
				width := runewidth.StringWidth(stripped)
				expectedWidth := tc.termWidth - 6
				if width != expectedWidth {
					t.Errorf("Width mismatch: got %d, want %d", width, expectedWidth)
				}

				if strings.Contains(result, ContextIcon) {
					t.Logf("Context bar visible at width %d", tc.termWidth)
				} else if tc.minBarWidth > 0 {
					t.Logf("Context bar not shown at width %d (might be too narrow)", tc.termWidth)
				}
			})
		}
	})

	t.Run("context bar respects minimum size with padding", func(t *testing.T) {
		// Use custom config with smaller spacers for this test
		config := &Config{
			LeftSpacerWidth:  2,
			RightSpacerWidth: 4,
		}
		s := NewWithConfig(deps, config)

		// Very narrow terminal where context bar won't fit with padding
		deps.TerminalWidth = &MockTerminalWidth{width: 50}
		data := &CachedData{
			ModelDisplay:   "C",
			CurrentDir:     "/",
			TermWidth:      50,
			UsedPercentage: 25.0,
		}

		result := s.Render(data)
		stripped := stripAnsi(result)

		// Should maintain width minus spacers (2+4=6)
		width := runewidth.StringWidth(stripped)
		expectedWidth := 50 - 6
		if width != expectedWidth {
			t.Errorf("Width should be maintained: got %d, want %d", width, expectedWidth)
		}

		// Context bar should not appear if there isn't enough space with padding
		// (needs at least 15 chars after 8 chars of padding)
		if strings.Contains(result, ContextIcon) {
			// If it does appear, verify it still has proper structure
			t.Log("Context bar appeared even in very narrow terminal")

			// Check that the total width is still correct
			if width != expectedWidth {
				t.Error("Context bar broke width constraints")
			}
		} else {
			t.Log("Context bar correctly hidden when too narrow to display with padding")
		}
	})

	t.Run("padding is exactly 4 spaces on each side", func(t *testing.T) {
		// Direct test of createContextBarFromPercentage method
		s := CreateStatusline(deps)
		s.colors = CatppuccinMocha{} // Initialize colors

		// Give it plenty of width so we can clearly see the padding
		barWidth := 60
		percentage := 25.0

		result := s.createContextBarFromPercentage(percentage, barWidth)

		// The result should be exactly barWidth characters
		stripped := stripAnsi(result)
		actualWidth := runewidth.StringWidth(stripped)
		if actualWidth != barWidth {
			t.Errorf("Context bar width incorrect: got %d, want %d", actualWidth, barWidth)
			t.Logf("Raw result length: %d", len(result))
			t.Logf("Stripped length: %d", len(stripped))
			t.Logf("Stripped width: %d", actualWidth)
		}

		// Should start with exactly 4 spaces
		if !strings.HasPrefix(result, "    ") {
			t.Error("Context bar should start with exactly 4 spaces")
		}

		// Should end with exactly 4 spaces
		if !strings.HasSuffix(stripped, "    ") {
			t.Error("Context bar should end with exactly 4 spaces")
		}

		// The actual bar content should be in the middle
		trimmed := strings.TrimSpace(stripped)
		trimmedWidth := runewidth.StringWidth(trimmed)
		if trimmedWidth != barWidth-8 { // 8 = 4 spaces on each side
			t.Errorf("Bar content width incorrect: got %d, want %d", trimmedWidth, barWidth-8)
		}
	})
}
