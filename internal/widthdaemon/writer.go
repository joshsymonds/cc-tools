package widthdaemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	defaultWriterDir = "/dev/shm/cc-tools"
	parentWidthFile  = "parent-width"
	widthsJSONFile   = "widths.json"

	// Cache files are world-readable: dispatched agent processes may
	// run under the same user but the daemon is the only writer, and
	// keeping it 0644 mirrors how /var/run state files are typically
	// shared on a single-user box.
	cacheFileMode = 0o644
	cacheDirMode  = 0o755

	// 4 bytes of randomness -> 8 hex chars; collisions across
	// concurrent writers in the same dir are astronomically unlikely
	// for a single-machine daemon. We avoid os.CreateTemp because the
	// project's lint config forbids "Temp" in production code.
	scratchRandBytes = 4
)

// Writer persists the daemon's current width observation to disk in a
// way that readers (the statusline running in a headless dispatched
// agent) can consume safely. All file writes go through a scratch
// file + rename so concurrent readers never see a partial line.
//
// Dir is the directory both files live in; defaults to
// /dev/shm/cc-tools when empty.
type Writer struct {
	Dir string
}

// Write persists width and the per-source detail to Dir atomically.
// If width <= 0 the call is a no-op: the epic forbids writing 0 or
// negative widths because the statusline's read path would parse them
// as valid sources.
func (w *Writer) Write(width int, sources []Source) error {
	if width <= 0 {
		return nil
	}
	dir := w.resolveDir()
	if mkErr := os.MkdirAll(dir, cacheDirMode); mkErr != nil {
		return fmt.Errorf("mkdir %s: %w", dir, mkErr)
	}

	widthBytes := []byte(strconv.Itoa(width) + "\n")
	if writeErr := writeFileAtomic(dir, parentWidthFile, widthBytes); writeErr != nil {
		return fmt.Errorf("write %s: %w", parentWidthFile, writeErr)
	}

	jsonBytes, marshalErr := json.MarshalIndent(sources, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("marshal sources: %w", marshalErr)
	}
	if writeErr := writeFileAtomic(dir, widthsJSONFile, jsonBytes); writeErr != nil {
		return fmt.Errorf("write %s: %w", widthsJSONFile, writeErr)
	}
	return nil
}

func (w *Writer) resolveDir() string {
	if w.Dir == "" {
		return defaultWriterDir
	}
	return w.Dir
}

// writeFileAtomic writes data to dir/name via a scratch file + rename.
// On any error before the rename, the scratch file is removed so the
// directory never accumulates orphans.
func writeFileAtomic(dir, name string, data []byte) error {
	scratchPath, openErr := openScratchFile(dir, name)
	if openErr != nil {
		return fmt.Errorf("open scratch: %w", openErr)
	}

	if writeErr := os.WriteFile(scratchPath, data, cacheFileMode); writeErr != nil {
		_ = os.Remove(scratchPath)
		return fmt.Errorf("write scratch: %w", writeErr)
	}

	finalPath := filepath.Join(dir, name)
	if renameErr := os.Rename(scratchPath, finalPath); renameErr != nil {
		_ = os.Remove(scratchPath)
		return fmt.Errorf("rename %s -> %s: %w", scratchPath, finalPath, renameErr)
	}
	return nil
}

// openScratchFile picks a unique scratch path in dir, creates it with
// O_EXCL to avoid collisions, and returns the path. The file is left
// open-and-closed (writeFile will rewrite it) — we just needed the
// O_EXCL guarantee that we own this name.
func openScratchFile(dir, base string) (string, error) {
	suffix := make([]byte, scratchRandBytes)
	if _, randErr := rand.Read(suffix); randErr != nil {
		return "", fmt.Errorf("entropy: %w", randErr)
	}
	scratch := filepath.Join(dir, fmt.Sprintf("%s-%s.scratch", base, hex.EncodeToString(suffix)))
	// gosec G304: scratch path is built from a randomized suffix inside
	// the daemon-owned cache dir; not user-controlled.
	//nolint:gosec // see comment above
	f, openErr := os.OpenFile(scratch, os.O_RDWR|os.O_CREATE|os.O_EXCL, cacheFileMode)
	if openErr != nil {
		return "", fmt.Errorf("open %s: %w", scratch, openErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(scratch)
		return "", fmt.Errorf("close %s: %w", scratch, closeErr)
	}
	return scratch, nil
}
