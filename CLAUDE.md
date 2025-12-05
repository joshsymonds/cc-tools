# cc-tools Development Guide

Go implementation of Claude Code hooks and utilities. Provides validation hooks (lint/test), statusline generation, and MCP management.

## Project Structure

```
cmd/
├── cc-tools/           # Main CLI with subcommands (validate, skip, debug, mcp, config)
├── cc-tools-statusline/  # Standalone statusline binary for Claude Code
└── cc-tools-validate/    # Standalone validation binary for Claude Code

internal/
├── config/       # JSON config management (~/.config/cc-tools/config.json)
├── debug/        # Debug logging control
├── hooks/        # Core hook logic: discovery, execution, locking, validation
├── mcp/          # MCP server enable/disable management
├── output/       # Terminal output formatting and tables
├── skipregistry/ # Per-directory skip tracking for lint/test
├── statusline/   # Statusline generation with Catppuccin colors
└── shared/       # Project detection, colors, debug paths
```

## Key Design Patterns

### Dependency Injection

All major components use constructor-injected dependencies for testability:

```go
// Hooks use Dependencies struct
type Dependencies struct {
    FS       FileSystem
    Runner   CommandRunner
    Process  ProcessChecker
    Clock    Clock
}

func NewCommandDiscovery(projectRoot string, timeout int, deps *Dependencies) *CommandDiscovery
func NewLockManager(workspaceDir, hookName string, cooldown int, deps *Dependencies) *LockManager

// Statusline uses its own Dependencies
type Dependencies struct {
    FileReader    FileReader
    CommandRunner CommandRunner
    EnvReader     EnvReader
    TerminalWidth TerminalWidth
    CacheDir      string
    CacheDuration time.Duration
}
```

### Command Discovery

Hooks discover project commands by walking up from edited file to project root, checking:
1. Makefile/makefile - `make lint`, `make test`
2. Justfile - `just lint`, `just test`
3. package.json - `npm/yarn/pnpm run lint|test`
4. scripts/ directory - `./scripts/lint`, `./scripts/test`
5. Language-specific: golangci-lint, ruff, pytest, cargo clippy, etc.

### Lock Management

PID-based locking prevents concurrent hook execution:
- Lock files in `/tmp/claude-hook-{name}-{workspace-hash}.lock`
- Cooldown period after completion prevents rapid re-execution
- Stale lock detection via PID existence check

### Exit Code Protocol

Claude Code hooks use specific exit codes:
- `0`: Silent success (lock unavailable, skipped)
- `2`: Show message to user (success or failure with feedback)

## Configuration

Config stored at `~/.config/cc-tools/config.json`:

```json
{
  "validate": {
    "timeout": 60,
    "cooldown": 5
  },
  "statusline": {
    "workspace": "",
    "cache_dir": "/dev/shm",
    "cache_seconds": 20
  }
}
```

## Testing Hooks Locally

```bash
# Test validate hook
echo '{"hook_event_name": "PostToolUse", "tool_name": "Edit", "tool_input": {"file_path": "main.go"}}' | ./build/cc-tools-validate

# Test statusline
echo '{"cwd": "'$(pwd)'", "model": {"display_name": "Claude"}}' | ./build/cc-tools-statusline

# Test skip functionality
./build/cc-tools skip lint
./build/cc-tools skip status
./build/cc-tools unskip

# Enable debug logging
./build/cc-tools debug enable
./build/cc-tools debug filename  # Shows log path
```

## Build Commands

```bash
make build    # Build all binaries to build/
make test     # Run tests with coverage
make lint     # Run gofmt, golangci-lint, deadcode
make install  # Copy binaries to ~/bin
```
