---
name: merlin-social
description: Use when the user wants Discord notifications, Slack posts, email marketing (Klaviyo audit, campaign creation, cold outbound), Reddit organic prospecting/drafting/posting, Threads posts, competitor ad intelligence via Meta Ad Library, or any non-paid social/community channel. Covers the 6 essential DTC email flows (welcome, cart, browse, post-purchase, win-back, sunset), first-touch/linear/time-decay attribution models, Reddit's 7-layer compliance preflight, and the weekly competitor ad scan with hook extraction.
owner: ryan
---

# Owned & Earned Channels

## Discord (`mcp__merlin__discord`)

| Action | Key params |
|---|---|
| `setup` | (changes channel) |
| `post` | `slackMessage` (reused field name) |

**Connect:** `platform_login({platform: "discord"})` — opens Discord's bot authorization, user picks server, bot auto-discovers text channels.

**Auto-posts:** When Discord is connected, Merlin posts automatically when ads are published, paused, scaled, or new creatives generated. No manual trigger needed for these.

## Email Marketing (`mcp__merlin__email`)

| Action | Key params |
|---|---|
| `audit` | `brand` — existing flows, lists, campaigns, missing essentials, recommendations |
| `revenue` | `brand` — attribution-based revenue per flow |

If Klaviyo isn't connected, tell the user to click the Klaviyo tile or paste an API key.

### Attribution models — always state which one

| Model | Use for |
|---|---|
| **First-touch** | "Which channel brought them in" — 100% credit to the first email that touched the buyer |
| **Linear** | Nurture-heavy journeys where every touch mattered — equal credit across every touch |
| **Time-decay** | **Default for per-flow ROI** — half-life 7 days, recent touches weighted higher |

Never mix models in the same report. "Is welcome series working?" → time-decay. "Is email worth investing in?" → first-touch.

### Essential DTC email flows

1. **Welcome Series** (3 emails / 5 days): Welcome + brand story → bestsellers showcase → social proof + first-purchase discount
2. **Abandoned Cart** (3 emails): Reminder (1hr) → social proof (24hr) → urgency/discount (48hr)
3. **Browse Abandonment** (2 emails): "Still looking?" (4hr) → related products (24hr)
4. **Post-Purchase** (3 emails): Thank you + order details → how to use/style → review request (14 days)
5. **Win-back** (3 emails): "We miss you" (60 days) → bestsellers update (75 days) → final discount (90 days)
6. **Sunset** (2 emails): "Still interested?" (90 days no opens) → final chance before suppression (120 days)

### Cold outbound benchmarks

- **40%+ open rate** — below this, subject line or sender reputation is broken
- **3%+ reply rate** — below this, body copy or CTA is off
- **1%+ positive reply rate** — below this, ICP or offer is wrong

Warm list (existing subscribers): 35% open / 2% click / <0.5% unsubscribe. Diagnose in that order — opens → envelope, clicks → body, unsubs → list hygiene or send frequency.

### Email template rules

- 600px wide (Klaviyo standard). Table-based HTML with inline styles.
- Use the real logo PNG (`logo/logo.png`), never AI-generated text.
- Use real product photos from Shopify CDN, never AI-generated product shots.
- Brand colors are exact hex codes from `brand.md` → Brand Colors section.

## Klaviyo (`mcp__merlin__klaviyo`)

`performance` · `lists` · `campaigns`

**Review solicitation pattern** (daily scheduled task): find fulfilled orders 5–7 days old (via `shopify-orders`), draft a Klaviyo campaign per order (max 3/day to avoid spam): product photo + "How are you liking your {product}?" + review link. Publish as draft — user or `merlin-optimize` approves.

## Reddit Organic

Organic-growth pipeline for finding pain-point threads, clustering them, drafting quality-gated replies, and (optionally) posting under heavy compliance preflight. Same OAuth + `redditAccessToken` as Reddit Ads.

| Action | Purpose | Key params |
|---|---|---|
| `reddit-prospect-scan` | Search Reddit for relevant threads | `brand`, `keywords`, `subreddits` (comma list or blank = sitewide), `scanLimit` |
| `reddit-prospect-draft` | Cluster pain points + draft quality-gated replies | `brand`, `keywords`, `subreddits`, `draftLimit`, `draftDryRun` |
| `reddit-prospect-post` | Submit ONE approved reply | `brand`, `threadId`, `subreddit`, `draftBody`, `approved: true` |
| `reddit-shadowban-check` | Authed /me vs unauth /user probe | `brand` |

### Quality gate (`reddit-prospect-draft`)

Every draft passes through:
1. **Structural gate** — 40–300 words, no banned self-promo phrases, no shouting, no URLs unless sub allows, must reference at least one thread-content token.
2. **Optional AI gate** — Gemini scores authenticity/helpfulness/thread-fit/compliance/overall; fails closed on network error.

Drafts below threshold are dropped, not surfaced.

### Compliance preflight (`reddit-prospect-post`, auto mode)

7 layers before `/api/comment`:
1. Account ≥30 days old
2. Combined karma ≥100
3. Max 5 posts / 24h
4. ≥120 min between posts to same subreddit
5. ≥300s between any two posts
6. Body not posted in last 72h (SHA-256 of normalized body)
7. No cached shadowban

Any block writes a `reddit_posts_<ts>.json` envelope with a friendly reason.

### `redditPostMode` config

| Value | Behavior |
|---|---|
| `"auto"` (default) | Full preflight → `/api/comment`. Gated on account age/karma/shadowban. |
| `"draft-only"` | Skip preflight + write. Reply saved to `results/reddit_draft_<ts>.txt` for manual paste. Zero shadowban risk, zero API writes. The legitimate path for fresh accounts / warm-up / shadowban recovery. |

When `auto` blocks on account-age / karma / shadowban, Merlin **saves the draft to disk anyway** as a fallback and surfaces `suggestion` hinting to switch to `draft-only`. When `auto` blocks on cadence or dedup (real spam signals), the draft is NOT saved — user waits or rewrites.

### Approval model

Every `reddit-prospect-post` surfaces an Electron approval card with sub + reply preview (200 chars). `approved: true` in the cmd envelope is defense-in-depth — the binary refuses to proceed without it.

## Threads (`mcp__merlin__threads`)

Connect via `platform_login({platform: "threads"})`. Post via threads action. Rate-limited separately from Meta Ads.

## Competitor Intelligence

### Discovery (onboarding + weekly digest)

**Step 1 — Infer from brand.** Read `brand.md` + product catalog → niche → WebSearch:
- `"<niche> brand" site:shopify.com`
- `"<category>" -[brand name]`
- Related brands on Instagram/TikTok in same niche

Find 5–8 competitors. For each: name, URL, product overlap, price range (cheaper / same / premium).

**Step 2 — Save to `assets/brands/<brand>/competitors.md`:**

```markdown
# Competitors — <Brand Name>
Discovered: YYYY-MM-DD

## Direct (same niche + price)
- **<Brand>** — <url> — <category>, $X-$Y

## Adjacent (overlapping audience)
- **<Brand>** — <url> — <category>, $X-$Y

## Aspirational (where the brand could grow toward)
- **<Brand>** — <url> — <category>, $X-$Y
```

**Step 3 — Weekly Ad Scan** (if `metaAccessToken` configured):

```json
{"action": "competitor-scan", "blogBody": "Madhappy,Pangaia,Teddy Fresh", "imageCount": 5}
```

Queries Meta Ad Library (UK/EU transparency — most US DTC brands run there too). Returns: `ad_creative_bodies`, `ad_creative_link_titles`, CTA captions, snapshot URL, publisher platforms.

Then:
1. Read each ad's copy → extract hooks, CTAs, offers.
2. WebFetch snapshot URLs to describe the visual creative.
3. Compare to recent ads.
4. Log insights to `memory.md` under `## Competitor Signals`.

**No Meta token** → fall back to WebSearch for competitor news.

**Limit:** Ad Library returns only ads that ran in UK/EU. Purely domestic US brands won't appear. Rate limit: 200 calls/hour.

### What to look for

- **Hook patterns**: "POV:", "Wait till you see...", "This changed everything"
- **Format trends**: video vs static, UGC vs polished, length
- **Script style**: conversational or scripted (read transcriptions)
- **Offer patterns**: free shipping, % off, BOGO, bundles
- **Running duration**: ads running 30+ days are proven winners — study these closely
- **New products**: anything we haven't seen before

### How this feeds back

- Heavy competitor video testimonials → try talking-head mode (route to `merlin-content`)
- Competitors running sales → consider value-focused angle instead of discounting
- Trending hook style → adapt for our brand voice
- Long-running competitor ads → reference their structure in our scripts
- Save winning patterns to `memory.md ## Competitor Signals`

## Slack

### Slack File Upload — 3-step (the ONLY method that works)

1. `GET https://slack.com/api/files.getUploadURLExternal?filename=X&length=Y` (query params, NOT JSON body)
2. `POST` raw file bytes to returned `upload_url`
3. `POST https://slack.com/api/files.completeUploadExternal` with JSON: `{files: [{id, title}], channel_id, initial_comment}`

`files.upload` and `files.uploadV2` are deprecated and will fail.

### Bot scopes

`channels:read`, `channels:join`, `files:read`, `files:write`, `chat:write`. `files:write` alone will upload but silently fail to share to channels.

### Posting rules

Post to both Slack + Discord if both configured. Activity notifications (ad published, killed, scaled) are automatic.

## Routing hints

- "connect discord" / "set up discord" / "discord channel" → `platform_login({platform: "discord"})`
- "email flows" / "klaviyo" / "welcome series" / "abandoned cart" → `email({action: "audit"})`
- "scan competitors" / "what are competitors running" → `competitor-scan` + Ad Library process
- "reply on reddit" / "post to reddit organically" → Reddit organic pipeline
- "post to slack" / "share to team" → Slack 3-step upload or `chat.postMessage`
