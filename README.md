# cc-tools

High-performance Go implementation of Claude Code utilities. Provides statusline generation and MCP management with minimal overhead.

## Features

### 📊 Rich Statusline
- **Model & cost tracking** - Current model, token usage, running costs
- **Git awareness** - Branch, dirty status, uncommitted file count
- **Environment context** - Kubernetes cluster, AWS profile, custom workspace
- **Visual indicators** - Token usage bars, color-coded states
- **Performance** - Cached results with 20-second refresh

### 🎛️ Development Controls
- **MCP management** - Enable/disable context servers per-project
- **Debug logging** - Detailed execution logs for troubleshooting
- **No daemon required** - Direct execution, no background processes

## Installation

### Claude Code Hooks

cc-tools provides the statusline hook that you can use in Claude Code itself.

- **`cc-tools-statusline`** - Generates the rich statusline

### Download Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/Veraticus/cc-tools/releases):

```bash
# Download and extract binaries
wget https://github.com/Veraticus/cc-tools/releases/latest/download/cc-tools-linux-amd64.tar.gz
tar -xzf cc-tools-linux-amd64.tar.gz

# Move to ~/.claude/bin/ (or any directory in your PATH)
mkdir -p ~/.claude/bin
mv cc-tools-statusline ~/.claude/bin/
chmod +x ~/.claude/bin/cc-tools-*
```

### Build from Source (NixOS)

```bash
# Build with Nix
nix-build

# Copy the required binaries
cp ./result/bin/cc-tools-statusline ~/.claude/bin/
```

### Build from Source (Go)

```bash
# Build all binaries
make build

# Copy the required binaries
cp build/cc-tools-statusline ~/.claude/bin/
```

### Claude Code Configuration

Add to your `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "~/.claude/bin/cc-tools-statusline",
    "padding": 0
  }
}
```

## Control Commands

The `cc-tools` binary provides control commands for managing your development workflow:

### Debug Logging

Enable detailed debug logging to troubleshoot hook behavior:

```bash
# Enable debug logging for current directory
cc-tools debug enable

# Check debug status
cc-tools debug status

# View log file path
cc-tools debug filename

# List all directories with debug enabled
cc-tools debug list

# Disable debug logging
cc-tools debug disable
```

### MCP Server Management

Control which MCP (Model Context Protocol) servers are active per-project:

```bash
# List all MCP servers and their status
cc-tools mcp list

# Enable specific MCP server
cc-tools mcp enable jira
cc-tools mcp enable playwright

# Disable specific MCP server
cc-tools mcp disable targetprocess

# Bulk operations
cc-tools mcp enable-all    # Enable all configured MCPs
cc-tools mcp disable-all   # Disable all MCPs (reduce context)
```

MCP names support flexible matching (e.g., 'target' matches 'targetprocess').

MCP management reads your existing MCP configurations from `~/.claude/settings.json`. Example configuration:

```json
{
  "mcpServers": {
    "playwright": {
      "type": "stdio",
      "command": "~/.claude/playwright-mcp-wrapper.sh",
      "args": [],
      "env": {}
    },
    "targetprocess": {
      "type": "stdio",
      "command": "~/.claude/bin/targetprocess-mcp",
      "args": [],
      "env": {}
    },
    "jira": {
      "type": "stdio",
      "command": "~/.claude/jira-mcp-wrapper.sh",
      "args": [],
      "env": {}
    }
  }
}
```

## Behavior

### Statusline

Generates a rich statusline for Claude Code prompts:

```bash
echo '{"cwd": "/path/to/project", "model": {"display_name": "Claude 3.5"}, "cost": {"input_tokens": 1000}}' | cc-tools statusline
```

Example output: image

## Configuration

All configuration is managed through the `cc-tools config` command. Settings are stored in `~/.config/cc-tools/config.json` and are automatically created with defaults on first use.

### Viewing Configuration

```bash
# List all settings with current values and defaults
cc-tools config list

# Example output:
# Configuration:
#   statusline:
#     - statusline.cache_dir = /dev/shm (default)
#     - statusline.cache_seconds = 20 (default)
#     - statusline.workspace =  (default)

# View the raw JSON config file
cc-tools config show

# Get a specific value
cc-tools config get statusline.cache_seconds
```

### Setting Configuration

```bash
# Set custom workspace label for statusline
cc-tools config set statusline.workspace "my-project"

# Set cache directory (e.g., for systems without /dev/shm)
cc-tools config set statusline.cache_dir "/tmp"
```

### Resetting to Defaults

```bash
# Reset a specific setting to its default
cc-tools config reset statusline.cache_seconds

# Reset all settings to defaults
cc-tools config reset
```

### Available Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `statusline.workspace` | "" | Custom label shown in statusline (e.g., project name) |
| `statusline.cache_dir` | /dev/shm | Directory for statusline cache files (fast tmpfs recommended) |
| `statusline.cache_seconds` | 20 | How long to cache statusline data before refreshing |

The `config list` command clearly shows which values are customized vs defaults, making it easy to see what you've changed from the standard configuration.

## Development

### Building

```bash
# Run tests
make test

# Run lints
make lint

# Build binary
make build

# Run all checks
make check
```

### Testing

```bash
# Unit tests
go test ./...

# With race detection
go test -race ./...

# Specific package
go test ./internal/statusline/...

# Verbose output
go test -v ./...
```

## License

MIT

## Author

Josh Symonds ([@Veraticus](https://github.com/Veraticus))
