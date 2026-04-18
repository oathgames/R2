---
name: clarify-intent
description: Use when the user's request is genuinely ambiguous and could route to multiple Merlin skills — e.g. "do something with my ads" (push? kill? insights?), "fix this" with no antecedent, "make it better" without an artifact reference, or a bare platform name ("meta") without a verb. Asks a 2-4 option chip question via AskUserQuestion before proceeding so Merlin never guesses wrong on high-stakes ad, budget, or content decisions.
owner: ryan
---

# Clarify Intent — Fallback Router

This skill fires when Claude can't confidently pick the right skill from the user's utterance. The goal is one crisp question, not a tutorial.

## When to invoke

Route here when ANY of these are true:

- The utterance contains a verb but no object: "push it," "kill them," "scale this" with no prior context in the conversation.
- The utterance contains an object but no verb: "meta," "my ads," "the campaign."
- A high-stakes word appears (kill, scale, budget, spend, delete, pause, publish) with ambiguous scope (one ad vs all ads, one platform vs all).
- The user references "it" / "this" / "them" and the antecedent is not in the last 10 turns.
- Two different skills claim the utterance with comparable confidence.

## What to ask

Use `AskUserQuestion` with 2–4 chip options. Rules:

- **2–4 word labels** on each chip. No full sentences.
- **One-sentence description** per chip explaining what will happen if picked.
- **Never echo the question as prose before the chips** — the chips ARE the question.
- **Include a "None of these" escape chip** only when the options may genuinely not cover the user's intent.

Examples:

| Ambiguous utterance | Chip options |
|---|---|
| "do something with my meta ads" | `Check performance` · `Publish new ad` · `Pause losers` · `Scale winner` |
| "fix my landing page" | `Audit & score it` · `Write new copy` · `Check speed/mobile` |
| "what about tiktok" | `Connect TikTok` · `TikTok performance` · `Push ad to TikTok` |
| "the winner" (no antecedent) | `Latest ad` · `Top product` · `Best blog post` · `Something else` |

## What NOT to do

- Do not ask "what would you like to do?" as open-ended text. Chips only.
- Do not pick a default and run — ambiguous + destructive = always ask.
- Do not load five different skills speculatively. Ask, then load one.
- Do not apologize for asking. Clarification is service, not friction.

## After the user picks

Route to the chosen skill's domain and let the SDK's semantic matching load the right SKILL.md. Do not summarize the chosen chip back to the user — just act.
