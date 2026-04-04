# Merlin — Development Workspace

You are building Merlin, not using it. Ryan is the developer.

## What is Merlin?

Merlin is an AI-powered CMO that runs as a desktop app (Electron + Claude Agent SDK). Users download a 6MB installer, open Merlin, and talk naturally — it handles ad creation, campaign management, performance optimization, email, SEO, and scaling. The Go binary does the heavy lifting (API calls, media processing), Claude orchestrates the workflow.

## Product Principles

- **UX so good a 5th grader can use it.** If it requires explanation, it's broken.
- **Zero manual tokens, zero API key hunting.** OAuth opens a browser, user clicks Authorize, done.
- **Security is production-grade.** Encrypted key storage, auth tokens, no secrets in plaintext, no credentials in git.
- **Every push must be release-quality.** No broken states, no half-finished features, no "we'll fix it later."
- **Fix source code, never work around it.** When testing breaks, fix the Go binary, rebuild, retest.

## Directory Structure

```
D:\autoCMO-claude\
├── autoCMO/             ← PUBLIC repo (oathgames/Merlin)
│   ├── app/             ← Electron desktop app (main.js, renderer.js, etc.)
│   ├── pwa/             ← Mobile PWA (stubbed, pending tunnel security)
│   ├── .claude/commands/ ← /merlin, /cmo, /r2, /update slash commands
│   ├── .claude/tools/   ← Binary + config (not in git)
│   ├── assets/brands/   ← Brand configs, reference photos
│   ├── CLAUDE.md        ← User-facing instructions
│   ├── package.json     ← Electron + SDK dependencies
│   └── version.json     ← Version tracking + update manifest
│
├── autocmo-core/        ← PRIVATE repo (oathgames/merlin-core)
│   ├── *.go             ← All Go source (meta, tiktok, fal, shopify, etc.)
│   ├── oauth.go         ← Universal OAuth + API key setup flows
│   ├── adbrief.go       ← Structured ad brief prompt builder
│   ├── bootstrapper/    ← Tiny installer that downloads the full app
│   ├── landing/         ← Landing page (Cloudflare Worker)
│   ├── wisdom-api/      ← Merlin's Wisdom (collective intelligence, Cloudflare Worker)
│   ├── .github/workflows/ ← CI: build + release pipeline
│   └── build.ps1        ← Local build script
│
└── autocmo-work/        ← TEST INSTANCE (not a git repo)
    ├── .claude/tools/   ← Compiled binary + real config with API keys
    ├── assets/brands/   ← Test brand data (MadChill)
    └── results/         ← Test output
```

## Build → Deploy → Test Loop

1. Edit source in `autocmo-core/`
2. Build: `cd autocmo-core && go build -o Merlin.exe .`
3. Copy: `cp autocmo-core/Merlin.exe autocmo-work/.claude/tools/Merlin.exe`
4. Test: `cd autocmo-work && .claude/tools/Merlin.exe --config .claude/tools/merlin-config.json --cmd '{...}'`
5. If it works → commit to autocmo-core, push
6. If it fails → fix source, rebuild, re-test

## Release Pipeline

1. Edit Go source in `autocmo-core/`, test from `autocmo-work/`
2. Tag: `cd autocmo-core && git tag v0.X.Y && git push --tags`
3. CI builds cross-platform binaries with `garble` (obfuscation) + bootstrapper installers
4. CI publishes to `oathgames/Merlin` GitHub Releases
5. Users: download bootstrapper from landing page → installs + launches Merlin
6. Existing users: auto-update toast detects new version → downloads silently → restart

## Version Bump Checklist (EVERY release)

When bumping a version, ALL of these must be updated. Missing ANY endpoint means users get stale links or broken updates:

1. `autoCMO/package.json` → `"version": "X.Y.Z"`
2. `autoCMO/version.json` → `"version": "X.Y.Z"` + `"notes"`
3. `autocmo-core/landing/index.html` → download links (`.dmg` and `.exe` filenames contain version)
4. Commit + push `autoCMO/` (public repo)
5. Commit + push `autocmo-core/` (private repo)
6. Tag `autocmo-core` with `vX.Y.Z` + push tag (triggers CI)
7. **Deploy landing page**: `cd autocmo-core/landing && npx wrangler deploy`
8. Verify: `curl -s https://merlingotme.com | grep -o "X\.Y\.Z"` returns the new version

Never skip step 7. The landing page is a Cloudflare Worker — git push does NOT auto-deploy it. You must run `wrangler deploy` separately.

## Sync Rules

- Changes to CLAUDE.md, commands, or version.json → sync to BOTH `autoCMO/` and `autocmo-work/`
- Never commit API keys, .env files, or merlin-config.json to git
- The `.gitignore` blocks: `*.go`, `*.exe`, `merlin-config.json`, `node_modules/`, `results/`
- Always verify both repos are clean before pushing: `grep -r "OLD_NAME" --include="*.go" --include="*.md"`

## Security Checklist (every push)

- [ ] No API keys or tokens in committed files
- [ ] No plaintext secrets — use `safeStorage` in Electron, encrypted config on disk
- [ ] OAuth tokens stored in memory only (session-scoped)
- [ ] WebSocket auth requires token handshake
- [ ] All user-facing errors are friendly — no stack traces, no raw JSON
- [ ] Approval cards show plain English, never tool names or command JSON

## Architecture Decisions

### Electron Desktop App
- Claude Agent SDK spawns Claude Code subprocess — uses user's existing Claude Pro/Max subscription
- No API key needed — SDK inherits auth from Claude Desktop installation
- `canUseTool` callback translates every tool call to plain English for approval cards
- Auto-update checks GitHub Releases every 4 hours, downloads silently, offers restart

### Go Binary (Merlin.exe)
- All platform API calls (Meta, TikTok, Shopify, fal.ai, etc.)
- OAuth flows with embedded credentials (garble-obfuscated)
- Structured AdBrief prompt builder for S-tier image generation
- Composite mode: real product cutout + AI scene for 100% product accuracy
- Background removal via fal-ai/birefnet

### Landing Page
- Cloudflare Worker at merlingotme.com
- Auto-detects OS for download button (Windows/Mac/Linux)
- Interactive savings calculator + demo overlay
- Cursor sparkle wand effect (desktop only)

### PWA Mobile (Stubbed)
- WebSocket bridge from phone → desktop Electron app
- QR code auth — scan to connect
- Pending: Cloudflare Tunnel for production security, approval sync between clients

## Current Test Brand
- Brand: MadChill (mad-chill.com) — streetwear
- Product: Sweatpants (product ID 7431710507085)
- Meta Ad Account: act_435598072824789
- Meta Page: 595992153596478
- Meta Pixel: 1149516046441530
- Meta App ID: 823058806852722 (pending App Review for Live mode)

## Binary Actions Reference
| Action | Description |
|---|---|
| `image` | Generate AI images via structured AdBrief |
| `meta-login` | One-click OAuth for Meta Ads |
| `meta-setup` | Create Testing/Scaling/Retargeting campaigns |
| `meta-push` | Upload image/video + create full ad |
| `meta-insights` | Pull yesterday's performance data |
| `meta-kill` | Pause an ad |
| `meta-duplicate` | Copy winner to scaling campaign |
| `meta-discover` | Auto-detect ad accounts, pages, pixels from token |
| `api-key-setup` | Open browser to provider's key page |
| `verify-key` | Validate an API key works |
| `tiktok-login` | One-click OAuth for TikTok (scaffolded) |
| `google-login` | One-click OAuth for Google Ads (scaffolded) |
| `shopify-login` | One-click OAuth for Shopify (scaffolded) |

## Known Issues / Blockers
- Meta App (823058806852722) is in Development Mode — ad creatives blocked until App Review passes
- No workaround for Meta dev mode — error subcode 1885183 is a hard platform restriction
- Seedance 2 not yet available via any API (HeyGen UI-only, fal.ai "coming soon")
