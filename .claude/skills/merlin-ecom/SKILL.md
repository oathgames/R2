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

**Inventory-aware ad pausing** — when called from the daily optimization loop, cross-reference inventory with active ads. If inventory ≤ 0, pause ads promoting that product. When restocked, re-enable ads that were paused for stock.

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

## Setup gotcha

First `stripe-setup` after connecting BOTH Shopify + Stripe prints a disambiguation prompt. Ask the user one question, then run `stripe-preference` with their answer before moving on. Default (no preference) prefers Shopify.

## What this skill does NOT cover

- **Ad platform revenue attribution** → `merlin-ads` + `merlin-analytics` (ROAS, ATC, purchases from pixel data).
- **Email revenue attribution** → `merlin-social` (Klaviyo first-touch / linear / time-decay models).
- **Blog / SEO revenue** → `merlin-seo` (organic sessions, rankings).
- **Revenue charts in the dashboard** → `merlin-analytics` reads `RevenueSource` outputs; this skill produces them.
