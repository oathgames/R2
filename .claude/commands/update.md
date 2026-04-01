---
name: update
description: Check for and install AutoCMO updates from GitHub. Updates generic files and binary while preserving your brand data, config, and memory.
user-invocable: true
---

You are the AutoCMO updater. Follow these steps exactly.

## Step 1: Check for updates

Fetch the remote version manifest:
```
WebFetch https://raw.githubusercontent.com/oathgames/AutoCMO/main/version.json
```

Read the local `version.json` in the project root. Compare versions.

If the remote version is the same or older, report "AutoCMO is up to date (vX.X.X)" and stop.

## Step 2: Back up current files

Create a backup directory: `.autocmo-backup/{local-version}/`

For each file listed in the remote `updatable` array that exists locally, copy it to the backup directory preserving the path structure.

Report: "Backed up {N} files to .autocmo-backup/{version}/"

## Step 3: Update generic files

For each file in the remote `updatable` array:
1. Fetch from `https://raw.githubusercontent.com/oathgames/AutoCMO/main/{path}`
2. Write to the local path, overwriting the existing file

**NEVER touch files matching patterns in the `preserve` array:**
- `memory.md` — user's learning memory
- `assets/brands/*/` — user's brand folders (except `example/`)
- `.claude/tools/autocmo-config.json` — user's API keys and config
- `results/` — user's generated content

## Step 4: Update the binary

Detect the platform:
- Windows: `AutoCMO-windows-amd64.exe`
- macOS ARM64: `AutoCMO-darwin-arm64`
- macOS Intel: `AutoCMO-darwin-amd64`
- Linux: `AutoCMO-linux-amd64`

Download the correct binary from the latest GitHub release:
```bash
curl -L -o .claude/tools/AutoCMO.exe "https://github.com/oathgames/AutoCMO/releases/latest/download/{binary-name}"
```

On macOS, also run:
```bash
xattr -d com.apple.quarantine .claude/tools/AutoCMO.exe
codesign --force --sign - .claude/tools/AutoCMO.exe
```

## Step 5: Report

```
AutoCMO updated: v{old} → v{new}

Updated files:
  ✓ CLAUDE.md
  ✓ .claude/commands/cmo.md
  ✓ (etc.)

Preserved (untouched):
  ✓ memory.md
  ✓ assets/brands/{user-brands}/
  ✓ .claude/tools/autocmo-config.json

Binary: ✓ AutoCMO.exe replaced

Backup: .autocmo-backup/{old-version}/

{release notes from version.json}
```
