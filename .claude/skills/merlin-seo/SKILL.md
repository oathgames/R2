---
name: merlin-seo
description: Use when the user wants to audit SEO, research keywords, check Google rankings, find content gaps vs competitors, write or publish blog posts, fix image alt text, plan editorial calendar, or build topic clusters. Covers the SEO Rubric (Impact × Confidence scoring), BOFU/MOFU/TOFU funnel classification, search intent matching (informational/navigational/commercial/transactional), E-E-A-T (Experience/Expertise/Authoritativeness/Trust) requirements, SERP feature targeting (featured snippets, PAA, image pack, video carousel), topic clusters (pillar + spoke), link authority reality checks (KD >30 needs inbound links), schema types (Product/FAQ/HowTo/Review/Breadcrumb), the striking-distance band (positions 4-20), and the blog generation pipeline with SERP-matched length, cannibalization check, mandatory internal linking, and Shopify publishing.
owner: ryan
---

# SEO & Blog Generation

## SEO (`mcp__merlin__seo`)

| Action | Key params | Purpose |
|---|---|---|
| `audit` | `brand` | Full SEO audit — indexability, meta, speed, schema |
| `keywords` | `brand` | Keyword research scored via the Rubric below |
| `rankings` | `brand` | Current Google Search Console positions |
| `gaps` | `brand` | Content gaps vs competitors |
| `fix-alt` | `brand`, `adId` (product ID), `campaignId` (image ID), `blogTitle` (alt text) | Safe alt text fix (never overwrites existing alt) |

## SEO Rubric

Score keywords on two 0–10 axes, then prioritize by **Impact × Confidence**.

**Impact factors:** search volume, commercial CPC, buyer-intent level, trend velocity (YoY).
**Confidence factors:** keyword difficulty, current ranking position, domain authority match, content-gap size.

### Funnel classification — tag every keyword

- **BOFU** — buying signals: "agency," "services," "pricing," "best X," "X vs Y," "alternative to," "hire," "buy," "review," "near me"
- **MOFU** — research signals: "how to," "guide to," "X strategy," "X template," "X checklist," "case study"
- **TOFU** — awareness: "what is," "why does," pure informational, no buying signal

Prioritize BOFU > MOFU > TOFU at equal Impact×Confidence — BOFU converts.

### Striking-distance band

Positions **4–20** in Google Search Console. These are the fastest wins — rank already exists, content already exists, small optimization often moves them into top 3. **Prioritize over net-new keywords unless Impact×Confidence is materially higher.**

### Search intent match (before writing a single word)

Classify every target keyword by intent:

- **Informational** — "how to," "what is," "why does" → article, guide, tutorial
- **Navigational** — "[brand] login," "[competitor] pricing" → don't try to rank for competitors' brand
- **Commercial investigation** — "best X," "X vs Y," "X review" → comparison page, roundup, versus post
- **Transactional** — "buy X," "X near me," "X free trial" → product / collection / category page, not a blog

**Intent-content mismatch is the #1 reason blog posts don't rank.** A "best CRM for startups" query returning a sales page gets demoted. Match the intent of the top 3 ranking results — if they're listicles, write a listicle.

### E-E-A-T (Experience, Expertise, Authoritativeness, Trust)

Since Google's December 2022 update, E-E-A-T is a material ranking factor, especially for YMYL (Your Money Your Life) topics. Every blog post must demonstrate:

- **Experience** — first-hand use / original data / case studies. "I tested 12 CRMs for 90 days" beats "here are the top 12 CRMs." AI-generated content without first-hand experience underperforms after the March 2024 core update. Surface real usage in every post.
- **Expertise** — author byline with credentials, linked bio, topical consistency across the author's posts.
- **Authoritativeness** — inbound links from recognized sources in the niche (see Link authority below).
- **Trust** — author contact, editorial policy, citations to primary sources, HTTPS, real business address in footer.

**Surface check before publishing:** post has a real author byline (not "Admin"), cites ≥2 primary sources for any factual claim, and includes ≥1 original element (data, screenshot, quote, experiment).

### SERP feature targeting

Ranking #1 organically is no longer the goal — owning the SERP is. Target these features:

- **Featured snippet** (position 0) — answer the query in 40–60 words directly below the H2 that matches the query. Use a definition, list, or table.
- **People Also Ask (PAA)** — include 3–5 question H2/H3s that match PAA questions for the primary keyword.
- **Image pack** — for commercial / product / how-to queries, include an original image with descriptive filename + alt text.
- **Video carousel** — for "how to" / tutorial queries, embed a 60–90s video with transcript.
- **Site links** — driven by site architecture + brand search volume; not per-page targetable.

Before writing: SERP-scrape the top 10 for the target keyword; note which features are present; plan to compete for each.

### Topic clusters (hub-and-spoke)

Google rewards topical authority, not isolated posts. Cluster every new post into a pillar:

- **Pillar page** — comprehensive 2,500–4,000 word guide on a broad topic (e.g. "CRM for startups"). Targets the high-volume head term.
- **Cluster posts** — 8–15 narrower posts linking up to the pillar (e.g. "CRM for 2-person startups," "free vs paid CRM," "CRM + Stripe"). Each targets a long-tail variant.
- **Internal linking** — every cluster post links up to the pillar with descriptive anchor; pillar links down to each cluster; clusters cross-link where natural.

A single isolated post ranks with 2–3× the effort vs. the same post inside a 10-post cluster. Plan clusters during editorial calendar, not per-post.

### Link authority reality check

For keywords with difficulty (KD) >30, content quality alone does not rank — links do. Content ranks ~40% of the way; domain authority + topical inbound links push the rest. Merlin does not automate link building (relationship work), but it MUST flag: *"Keyword difficulty is 42. Your referring domain count is ~15. This keyword typically requires 60+ referring domains to break top 10. Recommend a lower-KD target OR plan a parallel link-building initiative (HARO / Qwoted, guest posting, digital PR, podcast tours)."*

### Schema / structured data (beyond Article)

Add JSON-LD schema matched to content type — SERP CTR lift is typically 15–30%:

- **Product** — for collection / PDP pages (price, rating, availability)
- **FAQ** — for any post with ≥3 Q&A pairs
- **HowTo** — for step-by-step tutorials
- **Review / AggregateRating** — for comparison posts with rated entities
- **BreadcrumbList** — every non-home page
- **Organization** + **LocalBusiness** — homepage, contact page

Article schema is auto-injected (see Blog Generation step 4); other types must be added manually or via theme.

### Cadence

| Task | Frequency |
|---|---|
| Full SEO brief | Weekly |
| Striking-distance check | Daily (or before every content push) |
| Trend scout | 2×/week |
| Competitor gap analysis | Monthly |
| Backlink audit | Monthly |
| Core Web Vitals / technical SEO | Monthly |
| Cannibalization check | Before every new post |

## Blog Generation

When the user says "write a blog post" or the daily scheduled task triggers:

1. **Pick a topic** from the brand's products, recent ad winners (`memory.md`), or seasonal angles.

2. **Length — match to keyword difficulty and SERP, not a fixed number.** Before writing, check the word count of top 3 ranking results for the primary keyword and match it ±20%. General floors:
   - **Commercial investigation / "best X" posts (KD 20+)** — 1,500–2,500 words, lists + comparison tables.
   - **How-to / tutorial (KD 10–30)** — 1,000–1,800 words with step headings.
   - **Short-tail BOFU ("buy X," "[brand] review")** — 600–1,200 words; concision > bloat; conversion trumps length.
   - **Pillar pages** — 2,500–4,000 words (see Topic clusters above).
   - **News / seasonal hits** — 500–800 words; speed > depth.

   Then write in brand voice (`brand.md`):
   - Title with primary keyword (<60 chars, front-load the keyword)
   - Meta description 150–160 chars (see step 4)
   - H1 matches title; H2s match SERP's People Also Ask where possible
   - Casual, readable tone; Hemingway-level 7–9
   - Soft CTA linking to the product
   - App validates word count, keyword density, headings, meta length, internal linking before publishing
   - **Cannibalization check** — grep existing ranked URLs for the primary keyword. If another post already ranks, UPDATE that post instead of publishing a new one. Two posts fighting for the same keyword split authority and both underperform.

3. **Internal linking (mandatory):**
   - Featured product: `<a href="/products/{handle}">{Product Name}</a>`
   - 1–2 related products mentioned naturally
   - 1–2 previous posts (check via `blog-list` or memory.md)
   - Descriptive anchor text with keywords, **NOT "click here"**

4. **Meta description (mandatory):** 150–160 chars targeting primary keyword + value prop. Pass as `summary_html` (Shopify uses as excerpt + meta). App injects Article schema (JSON-LD) automatically.

5. **Generate featured image** via image pipeline (product-showcase style — see `merlin-content`).

6. **Publish:**
   ```json
   {"action": "blog-post", "blogTitle": "...", "blogBody": "<h2>...</h2>",
    "blogTags": "...", "blogImage": "path/to/featured.jpg"}
   ```

7. **Update `memory.md`** with: title, topic, date, URL, primary keyword.

### Topic rotation

- **Product spotlight** — deep dive + related links
- **Lifestyle/culture** — link 2–3 products
- **How-to-style** — 3–4 product links
- **Behind-the-brand** — flagship products

### Publish mode

Check `cfg.blogPublishMode`:
- `"draft"` → publish as draft: `{..., "draft": true}` (default — safer)
- `"published"` → publish live

### Fallback

If Shopify is not configured → save as `.html` in `results/` for manual posting.

## SEO Fix Queue

If `assets/brands/<brand>/seo.md` exists (generated by `seo-audit`):

- Fix 2–3 images with EMPTY alt text per run via `seo-fix-alt`.
- Mark each fixed item `[x]` in seo.md.
- **Never touch:** product titles, descriptions, prices, pages, theme.
- **Never overwrite existing alt text** — only fill empty ones.

## Cross-references

- **Copy Quality Gate** — apply from `merlin-content` before publishing any blog body.
- **Content Scoring (viral score)** — apply from `merlin-content` to the blog outline before writing.
- **Image generation** for featured images → `merlin-content`.
- **Competitor intelligence** feeds content gap detection → `merlin-social` for scan mechanics, this skill for interpretation.

## Routing hints

- "seo" / "rank" / "ranking" / "keyword" / "gap" → this skill.
- "write a blog" / "new post" → blog generation flow above.
- "fix alt" / "alt text" / "image descriptions" → `seo-fix-alt`.
- "compare to competitor rankings" → `seo-gaps` or cross-call to `merlin-social` for ad scan data.
