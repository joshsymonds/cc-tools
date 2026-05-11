package statusline

import "testing"

func TestAbbreviateModel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Sonnet 4.6 (1M Context)", "S4.6"},
		{"Opus 4.7 (1M Context)", "O4.7"},
		{"Haiku 4.5", "H4.5"},
		{"Sonnet 4.6 (1M Context, beta)", "S4.6"},
		{"Sonnet 4.6", "S4.6"},
		{"Claude", "Claude"},
		{"", "Claude"},
		// Multi-word name + version: take the LAST word's initial + the version.
		// `Claude 3.5 Sonnet` is the legacy v3 naming; we want S3.5.
		{"Claude 3.5 Sonnet", "S3.5"},
		// Unknown shape — keep raw (truncation handled later by render).
		{"GPT-4", "GPT-4"},
	}
	for _, c := range cases {
		if got := abbreviateModel(c.input); got != c.want {
			t.Errorf("abbreviateModel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
