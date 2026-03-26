# PortTracker

Manage your localhost ports. See which dev servers are running, from which project, and kill them when needed.

```
$ pt ls
PORT   PID    PROJECT     DIRECTORY                  COMMAND    UPTIME
3000   12607  my-app      ~/Projects/my-app          node       2h 13m
3001   13102  api         ~/Projects/api             node       45m
8081   14200  mobile      ~/Projects/mobile          metro      1h 02m
```

## Install

```bash
# Homebrew (macOS/Linux)
brew install yusufkaran/tap/porttracker

# Or download from releases
curl -fsSL https://github.com/yusufkaran/porttracker/releases/latest/download/install.sh | sh

# Or build from source
go install github.com/yusufkaran/porttracker/cmd/pt@latest
```

## Usage

```bash
pt              # Interactive TUI
pt ls           # List all listening ports
pt kill 3000    # Kill process on port 3000
pt kill my-app  # Kill all processes matching project name
pt 3000         # Shortcut for: pt kill 3000
```

### Interactive TUI

Run `pt` without arguments to open the interactive view:

```
⚡ PortTracker

PORT    PID     PROJECT              DIRECTORY                   COMMAND           UPTIME
3000    12607   my-app               ~/Projects/my-app           node              2h 13m
3001    13102   api                  ~/Projects/api              node              45m
8081    14200   mobile               ~/Projects/mobile           metro             1h 02m

↑↓ navigate • k kill • K kill project • o open • r refresh • q quit
```

### Project Detection

PortTracker automatically detects project names by reading:
- `package.json` (Node.js)
- `go.mod` (Go)
- `Cargo.toml` (Rust)
- `pyproject.toml` (Python)
- Falls back to directory name

## License

MIT
