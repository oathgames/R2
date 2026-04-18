---
name: merlin-tournament
description: Use when the user wants to iterate on a winning ad, generate rival variants, run a creative tournament, write better versions of an existing creative, beat their best ad, A/B candidates before pushing to Meta/TikTok, or otherwise produce > 1 challenger to an incumbent without LLM self-revision drift. Routes the four-role adversarial loop (critic teardown, blind author, synthesizer, blind 3-judge Borda) with the k=2 incumbent-wins-twice stop rule. NOT for fresh creative from scratch (use merlin-content) or for declaring statistical winners on already-running ads (use merlin-ads Promotion Gate).
owner: merlin-core
---

# Variant Tournament — adversarial creative iteration

## When to use this skill

The user has an incumbent creative — a live ad, a current best, a hero variant — and wants a *better* version. Not a fresh brief, not a statistical readout on what's running. They want challengers that could plausibly beat the incumbent, without the LLM-rewrites-its-own-work drift that produces 10 near-identical variations of the same hook.

Trigger phrases: *"make a better version of this ad," "iterate on this hook," "give me 3 challengers to my best Meta ad," "write rival variants and pick the winner," "tournament these creatives," "I want a stronger version of the pink-set ad."*

NOT for: fresh creative from a blank brief (`merlin-content`), declaring statistical winners on running ads (`merlin-ads` Promotion Gate), generic "write me 10 ad variations" (which produces drift — push back and route here instead).

## Why this exists (don't skip — it changes how you prompt)

Asking one LLM context to "write a better version" of its own (or any visible) creative produces three failure modes:
1. **Cosmetic drift** — the model rearranges words instead of changing the angle, because the incumbent text anchors the generation.
2. **Invented problems** — the model never says "this is already strong"; it manufactures critique to justify the rewrite, then "fixes" non-issues.
3. **Endless iteration** — without a stop rule, every round produces "improvements" that flatten brand voice and pull toward generic copy.

This skill structures a four-role loop where **no role sees what would bias it**, plus a hard stop when the incumbent wins twice in a row. The point is to widen the search space and then narrow it with a clean signal — not to keep editing.

## The four roles (orchestrate sequentially, isolate context)

You are running this in a single Claude conversation. "Isolation" means **what you put in each role's prompt** — not separate processes. Each role is a Task tool subagent invocation OR a clearly-fenced section of your own reasoning, where you DO NOT include the data the role isn't allowed to see.

### Role 1 — Critic (sees incumbent + knowledge layer)

Spawn a `sonnet` subagent (Task tool) with:
- The incumbent ad text (headline, body, CTA, format notes).
- The knowledge layer (see below).
- Instruction: *"Produce a teardown only. Do NOT write a replacement. Identify what is weak in the hook, what is generic in the body, what is missing compared to the swipe-file ads that convert at 3%+ CTR. If the incumbent is already strong on a dimension, say so explicitly — don't manufacture critique."*

Output: a structured teardown (hook score, body score, offer clarity, social proof, CTA friction, gaps vs swipe file). Save to `results/tournament_<timestamp>/round_<N>/teardown.md`.

### Role 2 — Author B (BLIND to incumbent)

Spawn a fresh `opus` subagent with:
- The teardown from Role 1.
- The knowledge layer.
- The original creative brief (product, audience, offer, format constraints).
- **NOT the incumbent text.** This is the load-bearing constraint. If Author B can see incumbent A, it will rearrange A's words. Withholding A forces a different generative trajectory.
- Instruction: *"Write a rival ad from scratch. The teardown describes weaknesses of an existing version you are NOT shown. Produce ONE complete creative — headline, body, CTA — that addresses those weaknesses while staying on brief and on brand. Pick a different angle from any swipe-file example, not a different wording."*

Output: rival creative B. Save to `round_<N>/rival_B.md`.

### Role 3 — Synthesizer (sees both)

Spawn `sonnet` with both A and B + the teardown:
- Produce three labeled candidates:
  - **A** — incumbent unchanged. (Always include. This is what makes the stop rule possible.)
  - **AB** — strongest hook from one + strongest body from the other, decided by which side of the teardown each excels at.
  - **B** — rival as-is.

Save to `round_<N>/candidates.json` with keys `{a, ab, b}`. Strip authorship metadata.

### Role 4 — Judge panel (3 BLIND judges, Borda count)

Spawn 3 separate `sonnet` subagents in parallel. Each gets:
- The three candidates with **anonymized labels** (`X`, `Y`, `Z` — re-shuffle the A/AB/B → X/Y/Z mapping per judge so position bias doesn't leak).
- The brand brief, knowledge layer, and swipe-file examples.
- Instruction: *"Score each on hook strength, offer clarity, social proof, CTA friction, and likely-CTR vs swipe-file benchmark. Rank 1st / 2nd / 3rd. You do NOT know which is original, which is rival, which is merge — score the artifact, not the source."*

Borda math: 1st = 2 points, 2nd = 1 point, 3rd = 0. Sum across 3 judges. Highest total wins. Ties broken by: more 1st-place votes → higher mean rank → reroll judge 3 with a fresh shuffle.

Save tallies to `round_<N>/borda.json`.

## The stop rule (k=2, non-negotiable)

After each round, the winning candidate becomes the new incumbent for the next round.

**Stop conditions (any one):**
- Incumbent A wins two rounds in a row (k=2). The system has converged — further iteration would be scope creep.
- Five rounds completed regardless of outcome (hard ceiling — runaway protection).
- User says stop.

When you stop because A won twice: report this as a feature, not a failure. *"The original ad won rounds 2 and 3 against fresh challengers. That's signal it's already strong — keep running it and rotate angle when fatigue shows in `merlin-ads` triage rule 3."*

When you stop because a challenger won the final round: push the new incumbent through the standard creative QA in `merlin-content` (Copy Quality Gate) before any `meta_ads({action: "push"})` call. The tournament narrows the search space — it does NOT replace QA, brand-voice check, or Promotion Gate stat-test once running.

## The knowledge layer (what to feed the roles)

Without grounding examples, the critic has no benchmark and the author writes copy that could be for any product. Before round 1, assemble:

1. **Performance history** — `mcp__merlin__dashboard({action: "dashboard", brand})` for current cross-platform numbers, plus the user's own `results/ad_*` and `results/img_*` folders for full creative history (each folder has a `metadata.json` with hook / format / verdict / ROAS tags). Pull the top 5 and bottom 5 by ROAS into every role so they know what wins and loses on this brand specifically. For collective benchmarks across all Merlin users in the same vertical, layer in `mcp__merlin__dashboard({action: "wisdom"})` — cite sample size when surfacing.
2. **Customer language** — read `assets/brands/<brand>/memory.md` and `brand.md` for voice samples, real customer quotes, support-ticket phrasings, review snippets. Feed verbatim — do not paraphrase.
3. **Competitor swipe file** — `mcp__merlin__competitor_spy({action: "ads-by-brand", foreplayBrandIds: "<ids>", foreplayLive: "true", foreplayOrder: "longest_running", limit: 20})` for 3-5 competitors. Longest-running = proven spend. This is the benchmark the critic and judges score against. If the user has no swipe file yet, prompt: *"To run the tournament with real benchmarks, point me at 3-5 competitors first — I'll pull their longest-running ads via competitor_spy. Cheaper than testing against a hunch."*
4. **Brand voice anchors** — `assets/brands/<brand>/brand.md` voice section + the offering noun from `vertical-profile`. Feeds judges' "on-brand" scoring dimension.

Cache the assembled knowledge layer to `results/tournament_<timestamp>/knowledge_layer.json` so every round in the same tournament sees the same baseline. Refresh swipe file every 30 days (competitor ad inventory shifts).

## Anti-drift guarantees (DO NOT bypass — these are why this works)

- **Author B never sees A.** If you put A in Author B's prompt "for context," you've defeated the skill. The whole point is generative independence.
- **Judges never see role labels.** If a judge knows which one is the original, anchoring takes over. Always anonymize and shuffle per-judge.
- **Critic does not write.** A teardown that includes "and here's how I'd rewrite it" pre-anchors Author B. Reject and re-spawn the critic with explicit "teardown only" if it volunteers a rewrite.
- **No memory between rounds beyond the new incumbent + tally.** Each round spawns fresh subagents. Do NOT pass round 1's teardown into round 2 — the new incumbent IS the input; the new round produces its own teardown.
- **The stop rule is not optional.** A loop without termination produces drift by definition. If incumbent wins twice, stop — even if the user asks for "one more round." Surface the convergence, ask if they want to change the brief or the swipe file instead.

## Output format (what the user sees)

After each round, render:

```
Round N — <winner-label> wins (Borda <total>)

Hook:    <winning hook line>
Body:    <winning body, truncated to 280 chars>
CTA:     <CTA>

Why it won (judge consensus): <one-sentence synthesis of the 3 judges' scoring rationale>
Margin: <total> vs runner-up <total> across 3 judges

Round file: results/tournament_<ts>/round_<N>/
```

Final round adds:

```
Tournament complete — <reason: A won twice | B wins after N rounds | hard ceiling>

Recommended action: <push to Promotion Gate testing in merlin-ads | keep incumbent | clarify brief>
```

## Implementation checklist (first time you run this for a brand)

1. Confirm the incumbent — *"Which ad is the one to beat? Paste the headline + body, or give me the ad ID and I'll pull it."*
2. Assemble knowledge layer (4 sources above). If swipe file is empty, prompt for competitors first — do NOT proceed without benchmarks.
3. Run rounds 1 → N with the four-role loop. Use the Task tool for subagent isolation (`opus` for Author B and the critic since the creative quality matters most; `sonnet` for synthesizer and judges to control cost).
4. Stop on k=2 or hard ceiling.
5. Hand the winner to `merlin-content` Copy Quality Gate before `merlin-ads` push.

## What this skill does NOT cover

- **Fresh creative from blank brief** → `merlin-content`
- **Declaring statistical winners on already-running ads** → `merlin-ads` Promotion Gate (Mann-Whitney U, p < 0.05, lift ≥ 15%, n ≥ 30)
- **Image/video generation** of the winning concept → `merlin-content` (image / video / heygen actions)
- **Pulling competitor ads** for the swipe file → `merlin-ads` (`competitor_spy` action) — this skill *consumes* the swipe file, doesn't fetch it
- **Performance teardowns of running ads** (CTR / CPA breakdowns) → `merlin-analytics`
