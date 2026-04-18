---
name: merlin-ads
description: Use when the user wants to push, publish, pause, kill, scale, duplicate, activate, lookalike, retarget, or set up paid ads on Meta, TikTok, Google Ads, Amazon, or Reddit Ads. Also covers ad performance decisions (promotion gate with Mann-Whitney U statistical test, kill thresholds, scale rules, fatigue detection), budget caps (maxDailyAdBudget / maxMonthlyAdSpend enforcement), the daily Meta autonomous loop (merlin-daily generate → merlin-optimize triage → merlin-digest weekly), ad intelligence rules (70/15/10 budget split, hook archetypes, format diversity over volume, learning phase gate), and the complete action reference for every ad platform.
owner: ryan
---

# Paid Advertising — All Platforms

## Universal rules (override Claude judgment on financial decisions)

- **Don't kill early.** Never pause/kill an ad in its first 72 hours unless CPM is 3× vertical average. Learning phase needs data.
- **Scale gradually.** Budget increases ≤20% of current daily, minimum 72 hours between increases. ROAS drops >15% after a budget increase → revert immediately.
- **Learning phase gate (Meta).** Before launching, check `(daily_budget / target_CPA) × 7 ≥ 50`. If not met: *"This campaign can't exit Meta's learning phase at this budget. Increase to $X/day or lower your target CPA."*
- **Budget split (new Meta setups).** 70% Advantage+ Shopping (ASC), 15% Retargeting, 10–15% Testing. **Don't A/B test creative inside ASC** — test in standard, move winners to ASC.
- **Format diversity over volume.** When a creative hits 2× ROAS, generate 5 new creatives in 5 different *formats* (UGC, product demo, lifestyle static, split-screen, meme) — not 5 variations of the same format.
- **Hook archetypes.** Every creative uses one: curiosity-gap, pattern-interrupt, problem-agitation, POV, social-proof-frontload, skit, before-after, direct-address, voiceover-demo, testimonial-open. Tag in `metadata.json`. QA rejects hooks <6/10 on attention pull.
- **Don't over-segment.** Brands under $1M/mo: one campaign with broad targeting and 10–15 creatives beats ten campaigns at $50/day each. The creative IS the targeting.
- **Don't test in ASC.** ASC optimizes delivery, not creative comparison. Always test in standard campaigns.
- **Budget caps.** Check `maxDailyAdBudget` and `maxMonthlyAdSpend` before ad spend. Stop if exceeded.

## Promotion Gate (stat-test before declaring winners)

Apply whenever Merlin would call something a "winner" or "loser" and act on it — moving ads from Testing → Scaling, killing creative, declaring an email subject winner, picking a landing page variant.

**Rule:** promote only if `p < 0.05` AND `lift ≥ 15%`. Both conditions. Either alone is noise.

**Test:** Mann-Whitney U (non-parametric, works with small samples, no normality assumption). Bootstrap confidence interval for the lift with 1,000 resamples.

**Minimum samples per variant:**
- **High-volume** (Meta/Google main campaigns): 10 conversions per variant
- **Low-volume** (email, retargeting, niche audiences): 30 conversions per variant

Below threshold → verdict is "keep running, insufficient data" — never "loser."

**Trending band:** `p < 0.10` with ≥15% lift = watch, don't kill. Early read without false positives.

Merlin's internal verdicts (KILL / WINNER / MASSIVE WINNER) already bake in spend/CPA heuristics. The promotion gate is the statistical ceiling — if a verdict says WINNER but the gate hasn't cleared, report both: *"flagged as winner by spend thresholds, but not yet statistically significant (p=0.14) — keep running before scaling."*

## Meta Ads Autonomous Loop

When Meta is configured:

```
Daily 9 AM (merlin-daily): generate content
  → 3 variations (batch) → Visual QA all 3 → push into ONE ad set in "Auto CMO - Testing"
  → Meta optimizes across the 3 creatives automatically

Daily 10 AM (merlin-optimize): review yesterday
  → Pull CTR/CPC/ATC/Purchases per ad → app evaluates against thresholds
  → Verdicts: KILL / WINNER / MASSIVE WINNER → act on these (apply Promotion Gate before scale)

Monday 9 AM (merlin-digest): weekly summary
  → Total spend/ATC/purchases/ROAS, best/worst, posted to Slack
```

**Two campaigns auto-created:**
- **Auto CMO - Testing** (ABO) — each ad gets its own budget. Isolated testing.
- **Auto CMO - Scaling** (CBO) — winners moved here. Meta optimizes across all winners.

### "Push to Meta" after approval

1. Check `maxDailyAdBudget` and `maxMonthlyAdSpend` in config.
2. Check `memory.md ## Monthly Spend` — at/over cap → warn and ask to confirm.
3. Upload image, create ad set + creative + ad in Testing campaign with `dailyBudget` capped at `maxDailyAdBudget`.
4. Report ad ID, link, daily budget.

### "Check Meta performance"

1. Pull yesterday's insights → table: spend, impressions, clicks, CTR, CPC.
2. Flag losers (KILL) and winners (SCALE per Promotion Gate).
3. Ask: *"Kill the losers and scale the winners?"*

### Meta platform gotchas

- **Live mode required for ad creatives.** Campaigns, ad sets, image uploads work in dev mode — only ad creative creation is blocked. Error subcode `1885183` = app in dev mode. **No workaround exists** (page tokens, system user tokens all fail).
- `metaFindCampaign` uses URL-encoded filtering to avoid duplicate campaign creation.
- `is_adset_budget_sharing_enabled` required on ALL campaigns (Meta v22.0+).
- CBO campaigns need `is_campaign_budget_optimization: true` + `daily_budget` at campaign level.
- On partial failure (creative/ad fails after ad set created), the ad set is auto-cleaned up.
- **Meta OAuth unavailable** — app is in App Review. Tell users to click the Meta tile in Connections and paste their token from `developers.facebook.com/tools/explorer`. Do NOT use `platform_login` for Meta.

## Triage rules (merlin-optimize)

Apply in order per ad. Daily budget derived from `cfg.dailyAdBudget` (default $20):
- `TESTING_BUDGET = DAILY_BUDGET × 0.60`
- `SCALING_BUDGET = DAILY_BUDGET × 0.30`
- `RETARGETING_BUDGET = DAILY_BUDGET × 0.10`
- `PER_AD_TEST_BUDGET = max($5, TESTING_BUDGET / active_test_count)`

| Rule | Condition | Action |
|---|---|---|
| 1 — Dead on arrival | spent ≥ 2× PER_AD_TEST_BUDGET AND purchases == 0 AND CTR < 1.0% | KILL |
| 2 — Low performer | spent ≥ PER_AD_TEST_BUDGET AND ROAS < 0.5 AND days_running ≥ 2 | KILL |
| 3 — Creative fatigue | days_running ≥ 5 AND CTR trend declining 30%+ from peak | KILL + queue replacement with DIFFERENT hook |
| 4 — Promising | days_running < 3 AND CTR ≥ 1.0% | HOLD |
| 5 — Winner | ROAS ≥ 1.5 AND days_running ≥ 2 AND spend ≥ PER_AD_TEST_BUDGET | SCALE (apply Promotion Gate before) |
| 6 — Massive winner | ROAS ≥ 3.0 AND spend ≥ DAILY_BUDGET AND purchases ≥ 5 | SCALE + LOOKALIKE (once per ad) |
| 7 — Retarget | Any WINNER exists AND retargeting has no active ads | Create retargeting ad with winner's creative |

**Safety rules:**
- **Aggregate kill cap:** never kill more than 50% of active ads in a single run.
- **Duplicate prevention:** before creating test ad, check `ads-live.json` — skip if same product + same hook already running.
- **Write-back:** update `ads-live.json` after every kill/scale/publish. Authoritative source of truth.

## Action Reference

### Meta Ads (`mcp__merlin__meta_ads`)

| Action | Key params |
|---|---|
| `push` | `adImagePath`, `adHeadline`, `adBody`, `dailyBudget` |
| `insights` | (none — pulls all active) |
| `kill` | `adId` |
| `activate` | `adId` or `campaignId` (status flip, NOT content creation) |
| `duplicate` | `adId`, `campaignId` (target campaign for scaling) |
| `setup` | (creates Testing + Scaling campaigns) |
| `lookalike` | `adId` (winner) |
| `retarget` | `adId` (winner) |
| `setup-retargeting` | (creates retargeting audiences) |
| `catalog` | (lists Facebook product catalog) |

### TikTok Ads (`mcp__merlin__tiktok_ads`)

`push` (`adVideoPath`, `adHeadline`, `adBody`, `dailyBudget`) · `insights` · `kill` (`adId`) · `duplicate` (`adId`, `campaignId`) · `setup` · `lookalike` (`adId`)

### Google Ads (`mcp__merlin__google_ads`)

| Action | Key params |
|---|---|
| `setup` | `brand` (creates "Merlin - Testing" $5/day + "Merlin - Scaling" $20/day Performance Max) |
| `push` | `adImagePath`, `adHeadline` (pipe-delimited), `adBody`, `adLink`, `dailyBudget` |
| `insights` | `brand` |
| `kill` | `campaignId` |
| `duplicate` | `campaignId` |

**Connect:** `platform_login({platform: "google", brand})` — OAuth, token + customer ID saved automatically.

### Amazon Ads (`mcp__merlin__amazon_ads`)

| Action | Key params |
|---|---|
| `setup` | `brand` |
| `push` | `campaignId`, `adGroupName`, `keywords` (array), `defaultBid` |
| `insights` | `brand` |
| `kill` | `campaignId` |
| `products` | `brand` (Seller catalog) |
| `orders` | `brand`, `batchCount` (days) |

**Connect:** `platform_login({platform: "amazon", brand})`.

### Reddit Ads (`mcp__merlin__reddit_ads`)

`campaigns` · `ads` · `insights` · `create` · `pause`

(Organic Reddit prospecting/drafting/posting is in `merlin-social`, not here.)

### Etsy (`mcp__merlin__etsy`)

Connector OAuth via `platform_login({platform: "etsy"})`. Same listing/insights pattern as above.

### LinkedIn (`mcp__merlin__linkedin`)

Connect + campaign ops. **Budget validation runs on the final scaled value** — if the code scales budget (e.g. 3× for LinkedIn scaling), `validateDailyBudget(cfg, scaledBudget, "linkedin")` must run on the scaled number. This is a regression guard — see `linkedin.go`.

## Rate limits

**Every outbound call to a rate-limited platform routes through `PreflightCheck` + `RecordSuccess`/`RecordRateLimitHit`.** A direct HTTP call to `graph.facebook.com`, `business-api.tiktok.com`, `googleads.googleapis.com`, `api.klaviyo.com`, `ads-api.reddit.com`, `openapi.etsy.com`, or an Amazon Ads host is blocked by the user-side hook. Always route through `mcp__merlin__*` tools.

## Routing hints

- "push" / "publish" / "launch" + platform name → platform's `push` action
- "kill" / "pause" / "stop" → platform's `kill`
- "scale" / "duplicate winner" → platform's `duplicate` or `lookalike`
- "catalog" / "products on facebook" → `meta_ads({action: "catalog"})`
- "insights" / "performance" on a specific platform → platform's `insights` (prefer `dashboard` for aggregate — see `merlin-analytics`)
- "set up" + platform → platform's `setup` action after OAuth

## What this skill does NOT cover

- **Ad creative generation** (images, videos, scripts) → `merlin-content`
- **Cross-platform aggregate performance** → `merlin-analytics`
- **Organic Reddit** / email / Discord → `merlin-social`
- **Shopify / Stripe revenue** → `merlin-ecom`
