package widthdaemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultWriterDir = "/dev/shm/cc-tools"
	parentWidthFile  = "parent-width"
	widthsJSONFile   = "widths.json"

	// Cache files are world-readable so other processes running as the
	// same user (the statusline binary in a dispatched-subagent
	// context) can consume them. The daemon is the only writer.
	cacheFileMode = 0o644

	// 0o700 (not 0o755) because no other UID has business reading
	// these files: dispatched subagents always run as the same user.
	// Tighter mode also serves as a sanity flag if a foreign UID
	// pre-created the dir on world-writable /dev/shm.
	cacheDirMode = 0o700

	// 4 bytes of randomness -> 8 hex chars; collisions across
	// concurrent writers in the same dir are astronomically unlikely
	// for a single-machine daemon. We avoid os.CreateTemp because the
	// project's lint config forbids "Temp" in production code.
	scratchRandBytes = 4
)

// errForeignCacheDir surfaces when the cache directory exists but is
// owned by a different UID. /dev/shm is world-writable (mode 1777),
// so a malicious local user could pre-create the dir and seed it with
// attacker-supplied widths. We refuse to write into a directory we
// don't own.
var errForeignCacheDir = errors.New("cache directory owned by another user")

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
//
// Refuses to write into a foreign-owned cache directory — a defense
// against squatting on world-writable /dev/shm.
func (w *Writer) Write(width int, sources []Source) error {
	if width <= 0 {
		return nil
	}
	dir := w.resolveDir()
	if mkErr := ensureOwnedDir(dir); mkErr != nil {
		return fmt.Errorf("prepare %s: %w", dir, mkErr)
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

// Heartbeat updates the cache files' mtime without rewriting content.
// Called by the daemon on every tick where the width is unchanged so
// downstream staleness checks (statusline's 5-min gate) can tell the
// daemon is alive even when the value has been stable for a long time.
//
// No-op (returns nil) when the cache files don't exist yet — the
// first Write will create them. Refuses to touch a foreign-owned
// cache directory for the same reason Write does.
func (w *Writer) Heartbeat() error {
	dir := w.resolveDir()
	if err := ensureOwnedDir(dir); err != nil {
		return fmt.Errorf("heartbeat prepare %s: %w", dir, err)
	}
	now := time.Now()
	for _, name := range []string{parentWidthFile, widthsJSONFile} {
		path := filepath.Join(dir, name)
		if err := os.Chtimes(path, now, now); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("heartbeat chtimes %s: %w", path, err)
		}
	}
	return nil
}

func (w *Writer) resolveDir() string {
	if w.Dir == "" {
		return defaultWriterDir
	}
	return w.Dir
}

// ensureOwnedDir creates dir if missing, then verifies the resulting
// directory is owned by the current UID. Refuses with
// errForeignCacheDir if a different user pre-created it (defending
// against /dev/shm squatting).
func ensureOwnedDir(dir string) error {
	if err := os.MkdirAll(dir, cacheDirMode); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	// Lstat (not Stat): if dir is itself a symlink to a foreign
	// location, that's also disqualifying.
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("lstat: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// Non-unix platform (or future Go change) — best-effort skip
		// the ownership check rather than fail closed.
		return nil
	}
	if int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("%w: dir=%s owner_uid=%d our_uid=%d",
			errForeignCacheDir, dir, stat.Uid, os.Getuid())
	}
	return nil
}

// writeFileAtomic writes data to dir/name via a scratch file + rename
// in a single open+write+close. On any error before the rename, the
// scratch file is removed so the directory never accumulates orphans.
//
// Single-open (no separate openScratchFile + os.WriteFile pair):
// O_EXCL on the scratch path guarantees nobody else owns that name
// for the duration of our write, and the 4-byte random suffix makes
// concurrent same-process collisions astronomically unlikely.
func writeFileAtomic(dir, name string, data []byte) error {
	scratchPath, scratchErr := scratchPathFor(dir, name)
	if scratchErr != nil {
		return fmt.Errorf("scratch path: %w", scratchErr)
	}

	//nolint:gosec // scratch path is built from a randomized suffix
	// inside the daemon-owned cache dir; not user-controlled.
	f, openErr := os.OpenFile(scratchPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		cacheFileMode)
	if openErr != nil {
		return fmt.Errorf("open scratch: %w", openErr)
	}
	if _, writeErr := f.Write(data); writeErr != nil {
		_ = f.Close()
		_ = os.Remove(scratchPath)
		return fmt.Errorf("write scratch: %w", writeErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(scratchPath)
		return fmt.Errorf("close scratch: %w", closeErr)
	}

	finalPath := filepath.Join(dir, name)
	if renameErr := os.Rename(scratchPath, finalPath); renameErr != nil {
		_ = os.Remove(scratchPath)
		return fmt.Errorf("rename %s -> %s: %w", scratchPath, finalPath, renameErr)
	}
	return nil
}

func scratchPathFor(dir, base string) (string, error) {
	suffix := make([]byte, scratchRandBytes)
	if _, randErr := rand.Read(suffix); randErr != nil {
		return "", fmt.Errorf("entropy: %w", randErr)
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s.scratch", base, hex.EncodeToString(suffix))), nil
}
