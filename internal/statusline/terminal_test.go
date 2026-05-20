package statusline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultTerminalWidth_GetWidth(t *testing.T) {
	tw := &DefaultTerminalWidth{}

	// Test with various environment setups
	tests := []struct {
		name     string
		setupEnv func(*testing.T)
		minWidth int
		maxWidth int
	}{
		{
			name: "with COLUMNS env var",
			setupEnv: func(t *testing.T) {
				t.Helper()
				t.Setenv("COLUMNS", "100")
			},
			minWidth: 80, // Could be 100 or fallback
			maxWidth: 150,
		},
		{
			name: "with CLAUDE_STATUSLINE_WIDTH env var",
			setupEnv: func(t *testing.T) {
				t.Helper()
				t.Setenv("CLAUDE_STATUSLINE_WIDTH", "120")
			},
			minWidth: 120,
			maxWidth: 120,
		},
		{
			name: "no env vars",
			setupEnv: func(_ *testing.T) {
				// t.Setenv automatically handles cleanup
			},
			minWidth: 80,   // Default fallback
			maxWidth: 1000, // Could get from terminal (covers ultrawide displays)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv(t)

			width := tw.GetWidth()

			if width < tt.minWidth || width > tt.maxWidth {
				t.Errorf("Expected width between %d and %d, got %d",
					tt.minWidth, tt.maxWidth, width)
			}
		})
	}
}

// pointWidthCacheAt swaps widthCacheFile to path for the test's
// lifetime and restores the original via t.Cleanup.
func pointWidthCacheAt(t *testing.T, path string) {
	t.Helper()
	original := widthCacheFile
	widthCacheFile = path
	t.Cleanup(func() { widthCacheFile = original })
}

func TestGetWidthCache_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parent-width")
	if err := os.WriteFile(path, []byte("123\n"), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	pointWidthCacheAt(t, path)

	tw := &DefaultTerminalWidth{}
	if got := tw.getWidthCache(); got != 123 {
		t.Errorf("getWidthCache = %d, want 123", got)
	}
}

func TestGetWidthCache_Missing(t *testing.T) {
	pointWidthCacheAt(t, "/nonexistent/cc-tools/parent-width")

	tw := &DefaultTerminalWidth{}
	if got := tw.getWidthCache(); got != 0 {
		t.Errorf("getWidthCache(missing) = %d, want 0", got)
	}
}

func TestGetWidthCache_Stale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parent-width")
	if err := os.WriteFile(path, []byte("80\n"), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	// Backdate mtime well past the stale window.
	stale := time.Now().Add(-2 * widthCacheStaleAfter)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	pointWidthCacheAt(t, path)

	tw := &DefaultTerminalWidth{}
	if got := tw.getWidthCache(); got != 0 {
		t.Errorf("getWidthCache(stale) = %d, want 0", got)
	}
}

func TestGetWidthCache_Garbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parent-width")
	if err := os.WriteFile(path, []byte("not a number\n"), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	pointWidthCacheAt(t, path)

	tw := &DefaultTerminalWidth{}
	if got := tw.getWidthCache(); got != 0 {
		t.Errorf("getWidthCache(garbage) = %d, want 0", got)
	}
}

func TestGetWidthCache_Zero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parent-width")
	if err := os.WriteFile(path, []byte("0\n"), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	pointWidthCacheAt(t, path)

	tw := &DefaultTerminalWidth{}
	if got := tw.getWidthCache(); got != 0 {
		t.Errorf("getWidthCache(0) = %d, want 0", got)
	}
}
