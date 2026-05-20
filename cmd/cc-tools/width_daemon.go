package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Veraticus/cc-tools/internal/widthdaemon"
)

const (
	widthDaemonDefaultActive = 1 * time.Second
	widthDaemonDefaultIdle   = 5 * time.Second
	widthDaemonDefaultStable = 30 * time.Second

	// Exit codes used only here so the linter doesn't flag literals.
	exitCodeFlagError = 2
	exitCodeRuntime   = 1
)

func runWidthDaemonCommand() {
	code := widthDaemonMain()
	if code != 0 {
		os.Exit(code)
	}
}

// widthDaemonMain is a separate function so its deferred cleanup runs
// before the outer os.Exit call (the gocritic exitAfterDefer rule).
func widthDaemonMain() int {
	fs := flag.NewFlagSet("width-daemon", flag.ContinueOnError)
	activeInterval := fs.Duration("active-interval", widthDaemonDefaultActive,
		"polling cadence after a recent width change")
	idleInterval := fs.Duration("idle-interval", widthDaemonDefaultIdle,
		"polling cadence once IdleAfter has elapsed without change")
	idleAfter := fs.Duration("idle-after", widthDaemonDefaultStable,
		"time without a change before backing off to IdleInterval")
	writerDir := fs.String("writer-dir", "",
		"directory for cache files (empty = /dev/shm/cc-tools)")

	if err := fs.Parse(os.Args[2:]); err != nil {
		return exitCodeFlagError
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := widthdaemon.Config{
		ActiveInterval: *activeInterval,
		IdleInterval:   *idleInterval,
		IdleAfter:      *idleAfter,
		WriterDir:      *writerDir,
	}

	_, _ = fmt.Fprintf(os.Stderr,
		"widthdaemon: starting active=%s idle=%s idle-after=%s dir=%s\n",
		cfg.ActiveInterval, cfg.IdleInterval, cfg.IdleAfter,
		resolveWriterDirForLog(cfg.WriterDir))

	if err := widthdaemon.Build(cfg).Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "widthdaemon: %v\n", err)
		return exitCodeRuntime
	}
	_, _ = fmt.Fprintln(os.Stderr, "widthdaemon: shutdown complete")
	return 0
}

func resolveWriterDirForLog(dir string) string {
	if dir == "" {
		return "/dev/shm/cc-tools (default)"
	}
	return dir
}
