# AutoCMO — Portable AI Content Engine

Open in Claude Code. Type `/cmo`. Everything else is automatic.

The `/cmo` command handles all setup on first run:
- Downloads the binary if missing
- Creates the config file if missing
- Asks for a fal.ai API key (the only requirement)
- Walks through brand + product setup
- Sets up daily automation if wanted

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
memory.md                       ← Learning memory (grows over time)
```

### Adding a new brand
Run `/cmo` — setup flow asks for website + writes brand.md.

### Adding a new product
Create a subfolder with `references/` inside a brand folder, drop photos in it. Claude auto-generates `product.md` on first use.

## Updates
Type `/update` to check for and install new versions.
Downloads the latest binary + framework files from GitHub while preserving user data (memory.md, brand folders, config).
Backups are saved to `.autocmo-backup/{version}/` before overwriting.

## How AutoCMO Improves Over Time
AutoCMO learns from anonymous, aggregated performance trends across all users.
When you check ad performance, AutoCMO contributes metrics like CTR and CPC
(never brand names, ad copy, or personal data) to improve recommendations for
everyone. This is what makes hook suggestions, format picks, and timing
recommendations smarter with every release.

## Key Rules
- Only `falApiKey` required to start. Everything else optional.
- Show cost estimate before running. Get confirmation.
- Show output inline before posting anywhere. `skipSlack: true` by default.
- Scheduled/automated runs skip confirmation.
- Memory compounds — every run improves the next.
- Brand-level assets (voice, avatar, quality bar) are shared across all products.
- Product-level assets (reference photos) are unique per item.
