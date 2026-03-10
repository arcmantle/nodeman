# nodeman

A fast, cross-platform Node.js version manager written in Go.

## Features

- **Install & manage** multiple Node.js versions from nodejs.org
- **Switch versions** instantly with shim-based forwarding (no shell hooks needed)
- **Shared global packages** — automatically reinstall tracked npm packages when switching versions
- **Cross-platform** — macOS (arm64/amd64), Linux (arm64/amd64), Windows (amd64)
- **Single binary** — no runtime dependencies
- **Proxy support** — respects `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`
- **Version file support** — reads `.nvmrc` and `.node-version` files
- **Self-upgrade** — update nodeman with a single command

## Quick Start

```bash
# Install nodeman (build from source)
go install github.com/roen/nodeman/cmd/nodeman@latest

# Create shims and configure PATH
nodeman setup

# Add to your shell profile (as printed by setup):
export PATH="$HOME/.nodeman/shims:$PATH"

# Install the latest LTS Node.js
nodeman install lts

# Set it as active
nodeman use lts

# Verify
node --version
```

## Commands

| Command | Description |
|---|---|
| `nodeman install <version>` | Download and install a Node.js version |
| `nodeman uninstall <version>` | Remove an installed version |
| `nodeman use [version]` | Set the active version (installs if needed) |
| `nodeman use --previous` | Switch back to the previously active version |
| `nodeman ls` | List installed versions |
| `nodeman ls-remote [--lts]` | List available versions from nodejs.org |
| `nodeman ls-remote --no-cache` | Bypass the 1-hour version cache |
| `nodeman current` | Show the active version |
| `nodeman setup` | Create shims, detect existing Node, validate PATH |
| `nodeman adopt` | Import existing system Node.js into nodeman |
| `nodeman doctor` | Diagnose configuration issues |
| `nodeman upgrade` | Upgrade nodeman to the latest release |
| `nodeman globals list` | List tracked global packages |
| `nodeman globals add <pkg>` | Track a global package |
| `nodeman globals remove <pkg>` | Untrack a global package |
| `nodeman completion <shell>` | Generate shell completions (bash/zsh/fish/powershell) |

## Version Specifiers

All commands that take a `<version>` argument accept flexible specifiers:

```bash
nodeman install 22          # Latest 22.x.x
nodeman install 22.14       # Latest 22.14.x
nodeman install 22.14.0     # Exact version
nodeman install lts          # Latest LTS release
nodeman install latest       # Latest overall release
```

## Version Files (.nvmrc / .node-version)

If you run `nodeman use` without a version argument, it searches for a `.nvmrc`
or `.node-version` file in the current directory and parent directories:

```bash
# Create a version file
echo "22" > .nvmrc

# nodeman reads it automatically
nodeman use
# Found /path/to/.nvmrc: 22
# Now using Node.js 22.14.0
```

## Environment Override

Set `NODEMAN_VERSION` to temporarily override the active version without
changing `config.json`:

```bash
NODEMAN_VERSION=20 node --version   # Uses latest installed 20.x
NODEMAN_VERSION=22.14.0 npm test    # Uses exact version
```

## Adopting Existing Installations

If you already have Node.js installed (via Homebrew, nvm, official installer, etc.),
nodeman can detect and import it:

```bash
# Scan and interactively adopt detected installations
nodeman adopt

# Adopt a specific version directly
nodeman adopt 22

# Adopt and immediately set as active
nodeman adopt --set-active 22
```

`nodeman setup` will also automatically detect existing installations and remind
you to adopt them. After adopting, you can safely uninstall the original
(e.g. `brew uninstall node`).

## Diagnostics

Run `nodeman doctor` to verify your setup:

```bash
nodeman doctor
# ✓ Data directory: /home/user/.nodeman
# ✓ Active version: 22.14.0
# ✓ 3 version(s) installed
# ✓ Shims directory exists
# ✓ Shims are on PATH
# ✓ node shim → Node.js v22.14.0
```

## Proxy Support

nodeman respects standard proxy environment variables:

```bash
export HTTP_PROXY=http://proxy.example.com:8080
export HTTPS_PROXY=http://proxy.example.com:8080
export NO_PROXY=localhost,127.0.0.1
```

All downloads (Node.js binaries, version index, checksums, self-upgrade) will
route through the configured proxy.

## Caching

Remote version listings are cached for 1 hour in `~/.nodeman/cache/`. Use
`--no-cache` with `ls-remote` to force a fresh fetch:

```bash
nodeman ls-remote --no-cache
```

## Self-Upgrade

```bash
nodeman upgrade
```

Downloads the latest release from GitHub and replaces the current binary.

## Shell Completions

Cobra provides built-in completion support. Add to your shell profile:

**Bash:**
```bash
echo 'eval "$(nodeman completion bash)"' >> ~/.bashrc
```

**Zsh:**
```bash
echo 'eval "$(nodeman completion zsh)"' >> ~/.zshrc
```

**Fish:**
```fish
nodeman completion fish | source
# To make persistent:
nodeman completion fish > ~/.config/fish/completions/nodeman.fish
```

**PowerShell:**
```powershell
nodeman completion powershell | Out-String | Invoke-Expression
# To make persistent, add the above to your $PROFILE
```

## How It Works

### Shims

When you run `nodeman setup`, it creates small shim binaries (`node`, `npm`, `npx`, `corepack`) in `~/.nodeman/shims/`. These are copies of the `nodeman` binary itself.

When invoked as `node` (or `npm`, etc.), `nodeman` detects the invocation name, reads the active version from `~/.nodeman/config.json`, and replaces itself with the real binary using `exec`.

This means:
- No shell hooks or `eval` needed
- Works with any shell (bash, zsh, fish, PowerShell, cmd)
- Zero overhead once exec'd — the shim is replaced by the real binary

### Global Packages

Track packages you want available across all Node.js versions:

```bash
nodeman globals add typescript eslint prettier
```

When you switch versions with `nodeman use`, all tracked packages are automatically reinstalled with the new version's npm.

## Directory Structure

```
~/.nodeman/
├── config.json        # Active version + previous version
├── globals.json       # Tracked global packages
├── cache/             # Cached remote version index
│   └── remote-versions.json
├── shims/             # Shim binaries (added to PATH)
│   ├── node
│   ├── npm
│   ├── npx
│   ├── corepack
│   └── nodeman
└── versions/          # Installed Node.js versions
    ├── 22.14.0/
    ├── 20.18.3/
    └── ...
```

## Building from Source

```bash
# Clone
git clone https://github.com/roen/nodeman.git
cd nodeman

# Build for current platform
make build

# Build for all platforms
make all

# Create a tagged release build
make release

# Output in dist/
ls dist/
```

## Cross-Compilation Targets

| OS | Architecture | Binary |
|---|---|---|
| macOS | arm64 | `nodeman-darwin-arm64` |
| macOS | amd64 | `nodeman-darwin-amd64` |
| Linux | arm64 | `nodeman-linux-arm64` |
| Linux | amd64 | `nodeman-linux-amd64` |
| Windows | amd64 | `nodeman-windows-amd64.exe` |

## License

MIT
