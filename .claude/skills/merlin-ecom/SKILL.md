---
name: merlin-ecom
description: Use when the user asks about Shopify products, orders, inventory, store analytics, Stripe revenue, MRR, ARR, churn, active subscribers, subscription cohorts, revenue source preference, or any e-commerce backend question. Handles Shopify product import, order batching, Stripe OAuth (read-only, never write), revenue aggregation with USD normalization, and the Shopify-vs-Stripe topline disambiguation when both are connected.
owner: ryan
---

# E-commerce Backend — Shopify + Stripe

## Shopify (`mcp__merlin__shopify`)

| Action | Key params | Purpose |
|---|---|---|
| `products` | `brand` | List products + inventory counts |
| `orders` | `brand`, `batchCount` (days) | Recent order data for revenue |
| `import` | `brand` | Pull all products + images into `assets/brands/<brand>/` |
| `cohorts` | `brand`, `batchCount` (days) | First-purchase cohorts → LTV, repeat rate |
| `analytics` | `brand`, `batchCount` (days) | 30-day window with AOV, ATC→purchase rate |
| `optimize-inventory` | `brand` | Cross-references Shopify inventory with live ads; pauses ads for OOS products, restores on restock |

**Inventory-aware ad pausing** — `optimize-inventory` walks `ads-live.json`, matches each live ad's `product` field to Shopify by slug, and pauses on 0 inventory. Paused ads are recorded in `.merlin-paused-for-stock.json` so the next run can auto-restore them when the product is restocked. Call this from the `merlin-optimize` scheduled task once per day. Safe to re-run; idempotent. Today only Meta is wired; TikTok/Reddit/LinkedIn stubs live in `inventory_sync.go:pausePlatformAd` — adding a case there wires the next platform.

**Shopify write boundary.** Never modify product titles, descriptions, prices, variants, sizes, inventory, pages, theme, or navigation. Merlin **adds** (blog posts, images) — never edits user content.

## Stripe (`mcp__merlin__stripe`)

OAuth-only, **read-only scope pinned to `read_only`**. Merlin cannot charge, refund, cancel, or modify anything. Enforcement is layered: (1) no write verbs in `stripe.go`, (2) Worker BFF re-verifies scope, (3) Stripe returns 403 on any write with a read-only token.

| Action | Key params | Purpose |
|---|---|---|
| `stripe-login` | `brand` | Opens browser OAuth — no API keys to paste |
| `stripe-setup` | `brand` | Verifies token, caches account ID |
| `stripe-preference` | `brand`, `provider` (`shopify` \| `stripe` \| `both`) | Disambiguates topline when both connected |
| `stripe-revenue` | `brand`, `batchCount` (days) | Gross, refunds, net, AOV, new customers (USD-normalized) |
| `stripe-subscriptions` | `brand` | MRR, ARR, active subs, 30-day churn, top plans |
| `stripe-cohorts` | `brand`, `batchCount` (days) | First-charge-month cohorts with lifetime revenue |
| `stripe-analytics` | `brand`, `batchCount` (days) | Consolidated revenue + subs + cohorts JSON |

**FX:** amounts USD-normalized via Stripe's `/v1/exchange_rates/usd`, cached 1 hour. Historical drift ±1%.

## DTC unit-economics benchmarks

Merlin-ecom surfaces the raw numbers; `merlin-analytics` layers MER and contribution margin on top. The benchmarks below flag health, not composite scores.

| Metric | Formula | Healthy | Unhealthy |
|---|---|---|---|
| **90-day repeat-purchase rate** | repeat customers (90d) / total customers (90d) | ≥25% DTC / ≥40% world-class | <15% — acquisition treadmill |
| **AOV** (average order value) | revenue / orders | trend direction > absolute | declining 3+ consecutive months |
| **ATC → purchase rate** | purchases / adds-to-cart | ≥30% healthy, ≥45% world-class | <20% = checkout friction |
| **Refund rate** | refunded orders / total orders | <5% DTC physical / <2% digital | >8% = product-expectation mismatch |
| **Bounce rate on PDP** | single-page sessions / total PDP sessions | <40% | >60% = ad/PDP mismatch |

## Subscription-economics benchmarks

When `stripe-subscriptions` or a subscription connector is active, surface these benchmarks:

| Metric | Formula | Healthy (B2C) | Healthy (B2B SaaS) | Why |
|---|---|---|---|---|
| **MRR growth** | (new + expansion − churn − contraction) / start MRR | ≥5%/mo early | ≥8%/mo | Topline momentum |
| **Net Revenue Retention (NRR)** | (start + expansion − churn − contraction) / start, 12mo cohort | ≥90% | ≥110% gold standard | The cleanest SaaS health metric |
| **Gross Revenue Retention (GRR)** | (start − churn − contraction) / start | ≥80% | ≥90% | NRR isolated from expansion |
| **Monthly churn** | churned subs / start-of-month subs | <5%/mo | <1%/mo | DTC subs tolerate higher churn; B2B does not |
| **Annual revenue churn** | 1 − (1 − monthly_churn)^12 | <45% B2C | <10% B2B | 5%/mo compounds to 46%/yr |
| **Quick Ratio** | (new + expansion) / (churned + contraction) | ≥2.0 | ≥4.0 | Efficiency of growth vs erosion |
| **CAC payback (subs)** | CAC / (ARPU × gross margin) | ≤12 months | ≤18 months | Cash recycle speed |
| **Expansion revenue share** | expansion MRR / total new MRR | ≥20% | ≥30% | Net-new vs upsell balance |

**Flagging rules:**
- NRR <90% → gross churn is eating growth. Diagnose retention before recommending more acquisition spend.
- Quick ratio <1.5 → subs leak faster than they fill. Stop the leak before pouring more in.
- Monthly churn >7% DTC / >2% B2B → product-retention issue, not a marketing issue. Recommend onboarding / engagement work before ad scale.

## Revenue source routing

When BOTH Shopify AND Stripe are connected AND Stripe is the Shopify payment processor, orders double-count without intervention. The `RevenueSource` abstraction (`dashboard.go`) gives every connector a uniform `{Name, Kind, Revenue, NewCustomers, MRR, ARR, ActiveSubs, ChurnPct}` shape; `pickRevenueSource` resolves the topline using `cfg.RevenueSourcePreference`.

| Preference | Topline behavior |
|---|---|
| `"shopify"` | Shopify revenue is topline, Stripe hidden from revenue row |
| `"stripe"` | Stripe revenue is topline, Shopify hidden from revenue row |
| `"both"` | Sum both — use only when they represent truly separate streams (e.g. physical DTC on Shopify + SaaS subscription on Stripe Billing) |
| `""` (default) | Prefer Shopify when both connected — safer, avoids double-count |

**Subscription metrics (MRR/ARR/churn) ALWAYS come from Stripe**, regardless of the topline pick. Shopify doesn't model subscriptions; Stripe does.

## Routing hints

- "how much revenue" / "what did I make" / "sales" → `dashboard` (which internally calls both and applies preference). Do NOT call `stripe-revenue` directly for topline questions.
- "MRR" / "ARR" / "churn" / "active subscribers" → `stripe-subscriptions` directly.
- "cohort" / "retention by month" / "LTV" → `stripe-cohorts` (subscription) OR `shopify-cohorts` (DTC).
- "out of stock" / "inventory" / "how much left" → `shopify-products` (inventory field).
- "pause ads for out-of-stock products" / "are we running ads for sold-out stuff" → `optimize-inventory`.

## Setup gotcha

First `stripe-setup` after connecting BOTH Shopify + Stripe prints a disambiguation prompt. Ask the user one question, then run `stripe-preference` with their answer before moving on. Default (no preference) prefers Shopify.

## What this skill does NOT cover

- **Ad platform revenue attribution** → `merlin-ads` + `merlin-analytics` (ROAS, ATC, purchases from pixel data).
- **Email revenue attribution** → `merlin-social` (Klaviyo first-touch / linear / time-decay models).
- **Blog / SEO revenue** → `merlin-seo` (organic sessions, rankings).
- **Revenue charts in the dashboard** → `merlin-analytics` reads `RevenueSource` outputs; this skill produces them.
