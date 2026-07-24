# lazyaddons

A cross-platform terminal UI for managing World of Warcraft addons via git.
Install, update, rollback — all from your terminal. Linux, macOS, and Windows.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)
![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

---

## Features

- **Add from any git URL** — GitHub, GitLab, self-hosted repos
- **Release or branch tracking** — auto-detects GitHub releases, lets you choose
- **Automatic update checks** — on startup and on demand (`u` key)
- **One-key updates** — press `Enter` to pull and re-unpack
- **Smart unpacking** — handles repos with nested addon folders, sub-modules, and junk files
- **Persistent config** — JSON config with atomic writes, XDG-compliant paths
- **Version + date display** — reads `.toc` version and last commit date (no API calls)
- **Self-update** — checks GitHub Releases on startup, one-key upgrade to latest version
- **Cross-platform** — `filepath`-safe, handles Windows drive letters and Wine prefixes
- **Scrollable list with search** — `/` to filter addons by name in real time
- **Styled keybinds** — keybinds highlighted in blue for better visibility

---

## Installation

### Install Script (recommended)

```bash
# Linux / macOS
curl -sSL https://raw.githubusercontent.com/pentsec/lazyaddons/main/install.sh | bash

# Install to a custom prefix if required
./install.sh --prefix ~/.local

# Install a specific version
./install.sh --version v1.0.0
```

* Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/pentsec/lazyaddons/main/install.ps1 | iex
```

### Build from Source

```bash
git clone https://github.com/pentsec/lazyaddons.git
cd lazyaddons
go build -o lazyaddons ./cmd/lazyaddons/
```

**Requirements**: Go 1.26+ (build from source only — pre-built binaries need nothing).

---

## Usage

### First Run

```bash
./lazyaddons

# or

lazyaddons
```

The first time, it asks for your WoW AddOns folder path. Examples:

| OS | Typical path |
|---|---|
| Linux (Wine) | `/home/user/.wine/drive_c/Program Files (x86)/World of Warcraft/Interface/AddOns` |
| Linux (native dir) | `/mnt/games/WoW/Interface/AddOns` |
| Windows | `C:\Program Files (x86)\World of Warcraft\_retail_\Interface\AddOns` |

### Version Check

```bash
lazyaddons --version
```

### Adding an Addon

1. Press **`a`** to open the add form
2. Paste the git URL (HTTPS or SSH)
3. If the repo has GitHub releases, choose a tracking mode:
   - **`1`** — Track `main` branch (follows commits)
   - **`2`** — Track latest release (follows tags)
4. The tool clones, unpacks, and adds it to the list

### Keyboard Shortcuts

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` | Navigate addon list |
| `/` | Search/filter addons by name |
| `a` | Add new addon |
| `d` | Delete addon (removes files + tracking) |
| `u` | Check for addon updates (all tracked) |
| `Enter` | Apply update (on selected addon with `↑` badge) |
| `U` | Self-update lazyaddons (when new version available) |
| `p` | Switch profile |
| `q` / `Esc` | Quit / cancel |

### Status Badges

| Badge | Meaning |
|---|---|
| `✓ up to date` | Addon is current |
| `↑ update avail` | New commits/release available |
| `✗ error` | Check failed (offline, missing repo) |

---

## How It Works

lazyaddons clones each addon repo into a **`.repo` directory** inside your WoW
AddOns folder, then unpacks the actual addon folders (the ones containing `.toc`
files) to the AddOns root so WoW can discover them.

```
Interface/AddOns/
├── Details/              ← addon files (what WoW loads)
│   └── Details.toc
├── Details.repo/         ← git repository (.git lives here)
│   └── .git/
├── Details_Options/      ← sub-module (included addon)
│   └── Details_Options.toc
└── Details_ASD/
    └── Details_ASD.toc
```

On update: deletes the unpacked folders, runs `git fetch` + `git merge --ff-only`
inside `.repo/`, and re-unpacks fresh copies. The `.git` directory never leaves `.repo/`.

The addon name is derived from the `.toc`-bearing folder inside the repo, not from
the URL. A repo `CleanerChat-WotLK` whose addon folder is `CleanerChat` will be
tracked as `CleanerChat`.

---

## Configuration

Config is stored at:

| OS | Path |
|---|---|
| Linux | `~/.config/lazyaddons/config.json` |
| macOS | `~/Library/Application Support/lazyaddons/config.json` |
| Windows | `%APPDATA%\lazyaddons\config.json` |

Example:

```json
{
  "version": 1,
  "wow_path": "/mnt/games/WoW/Interface/AddOns",
  "addons": [
    {
      "name": "DragonUI",
      "url": "https://github.com/PentSec/DragonUI",
      "track_mode": "release",
      "track_target": "v3.0.5",
      "current_sha": "",
      "version": "3.0.5",
      "last_updated": "2026-07-15"
    }
  ]
}
```

---

## Releases

Releases are automated via [GoReleaser](https://goreleaser.com) and GitHub Actions.
Pushing a tag triggers the build pipeline:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Produces: Linux, macOS, and Windows binaries for amd64/arm64, plus `checksums.txt`.

---

## License

[MIT](LICENSE)
