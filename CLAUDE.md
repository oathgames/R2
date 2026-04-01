# AutoCMO — Portable AI Content Engine

Open in Claude Code. Type `/cmo`.

## Session Protocol

### On Start
1. Scan `assets/` for brands and products
2. Read active brand's `brand.md` + product's `product.md`
3. Read `memory.md` — past learnings

### On Every Run
1. Resolve brand + product from user's request
2. Load brand.md + product.md + reference photos + quality benchmarks
3. After pipeline → show output inline, get approval before posting
4. After approval → update `memory.md`

## Folder Structure

```
assets/
└── <brand>/                    ← e.g., "madchill"
    ├── brand.md                ← Brand voice, audience, CTA (auto-generated)
    ├── quality-benchmark/      ← S-tier ad examples (quality bar)
    ├── voices/                 ← Voice samples for cloning
    ├── avatars/                ← Creator faces/videos
    └── products/
        ├── <product>/          ← e.g., "full-zip"
        │   ├── references/     ← Product photos (auto-pulled from store)
        │   └── product.md      ← Product details (auto-generated)
        └── <product>/
            ├── references/
            └── product.md

results/                        ← All output (timestamped)
memory.md                   ← Learning memory (grows over time)
```

### Adding a new brand
Create a folder under `assets/`, run `/cmo` — setup flow asks for website + writes brand.md.

### Adding a new product
Create a subfolder with `references/` inside a brand folder, drop photos in it. Claude auto-generates `product.md` on first use.

## macOS First-Run
If the user is on macOS and the pipeline binary is blocked by Gatekeeper, run:
```bash
xattr -d com.apple.quarantine .claude/tools/AutoCMO
codesign --force --sign - .claude/tools/AutoCMO
```
This removes the quarantine flag and ad-hoc signs the binary. Do this automatically before the first pipeline run if on macOS.

## Updates
Run `{"action": "update"}` to check for and install new versions automatically.
The binary downloads the latest release from GitHub and replaces itself.

## Key Rules
- Only `falApiKey` required to start. Everything else optional.
- Show cost estimate before running. Get confirmation.
- Show output inline before posting anywhere. `skipSlack: true` by default.
- Scheduled/automated runs skip confirmation.
- Memory compounds — every run improves the next.
- Brand-level assets (voice, avatar, quality bar) are shared across all products.
- Product-level assets (reference photos) are unique per item.
