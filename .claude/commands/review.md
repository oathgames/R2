---
allowed-tools: Agent, Bash(go build *), Bash(go test *), Bash(go vet *), Bash(staticcheck *), Bash(git *)
description: Adversarial Security Panel Review for Merlin
---

# /review - Adversarial Security Panel Review

Multi-phase code review using independent expert subagents with adversarial validation. Security-first: every review includes secrets scanning, encryption verification, and attack surface analysis. Targets 99% actionable fix rate.

**Merlin context**: This reviews Go source (autocmo-core/), Electron app (autoCMO/), Cloudflare Workers (landing/, wisdom-api/), and deployment configs. The binary handles 13 OAuth providers, AES-256-GCM vault, HMAC-signed rate limiting, and real money (ad spend). Security failures = leaked credentials, unauthorized ad spend, or customer data exposure.

## Batch Review Mode: `/review --batch`

When `$ARGUMENTS` contains `--batch`, review ALL pending git changes as a single atomic unit.

### Batch Step 1: Gather All Changes
```bash
git status --short 2>/dev/null
git diff --name-only HEAD 2>/dev/null
```
Parse all modified/added files. Group by system:
- **Go source** (autocmo-core/*.go)
- **Electron app** (autoCMO/app/)
- **Workers** (landing/, wisdom-api/)
- **Config/deployment** (.claude/, build/, .github/)

### Batch Step 2: Risk Classification
Classify the BATCH risk (not individual files):
- If ANY file is HIGH risk → entire batch is HIGH
- If 5+ files touched → at least MEDIUM
- Otherwise → aggregate individual risk levels

### Batch Step 3: Parallel Domain Reviews
Launch review agents grouped by domain:
- 1 Opus agent for all Go source changes (Security + Veteran lens)
- 1 Opus agent for all Electron/Worker changes (Security + UX lens)
- 1 Opus agent for cross-cutting concerns (Performance + Security lens)

Each agent reviews ALL files in their domain looking for:
- Cross-file consistency (do changes in oauth.go conflict with vault.go?)
- Secret leakage across boundaries
- Naming/convention consistency across the batch

### Batch Step 4: Conflict Detection
Check if any files were modified with conflicting intent:
- Same file, different changes → **CONFLICT** (present both diffs)
- Same file, same changes → **DEDUP**
- Different files in same system → cross-check for consistency

### Batch Step 5: Batch Verdict
```
╔══════════════════════════════════════════════════════════════════════════╗
║  BATCH REVIEW — N changes across M files                              ║
╠══════════════════════════════════════════════════════════════════════════╣
║  Risk: [overall risk level]                                            ║
║                                                                        ║
║  Go Source (K files):    SHIP IT / N issues                            ║
║  Electron/Workers (L):   SHIP IT / N issues                           ║
║  Cross-cutting:          SHIP IT / N issues                            ║
║                                                                        ║
║  SECURITY GATE:          PASS / FAIL                                   ║
║  Secrets scan:           CLEAN / N findings                            ║
║  Vault integrity:        VERIFIED / N issues                           ║
║                                                                        ║
║  Verdict: SHIP BATCH / FIX N ISSUES FIRST                             ║
╚══════════════════════════════════════════════════════════════════════════╝
```

---

## Phase 0: Risk Classification & Context Gathering

### Step 0.0 — Risk Classification

Before launching any agents, classify the change:

**LOW RISK:** Single-file content change, comment/doc-only, cosmetic rename/formatting, single-field value tweak in working code, brand asset changes.

**MEDIUM RISK:** Multi-file code change within one system, new function/method in existing file, bug fix changing control flow, config schema changes, new API action handler.

**HIGH RISK:** OAuth/auth changes (oauth.go, vault.go, any token handling), credential vault (AES-256-GCM, key derivation), rate limiting (ratelimit.go, ratelimit_preflight.go, HMAC), networking/WebSocket changes, Electron preload/IPC, Cloudflare Worker auth endpoints, CI/CD pipeline changes, any file touching secrets/tokens/keys, 4+ files touched, new platform integration.

Announce the risk level, then proceed to the matching tier.

### Step 0.1 — Security Context

Read these files for security baseline:
- `autoCMO/.claude/hooks/block-api-bypass.js` (what's blocked)
- `autocmo-core/vault.go` (encryption patterns)
- `autocmo-core/ratelimit_preflight.go` (HMAC patterns)
- `autocmo-core/oauth.go` (auth patterns)

### LOW RISK → Quick Review (1 agent, ~30 seconds)

Launch **one Opus agent** with combined expertise of all four reviewers.

**Quick Reviewer Prompt:** You are reviewing a low-risk change to Merlin (AI-powered CMO app) with the combined lens of: a security expert (secrets, encryption, auth), a senior Go engineer (error handling, concurrency, API design), a performance specialist (rate limiting, HTTP efficiency), and a UX expert (user-facing clarity, error messages). The app handles real ad spend money and OAuth tokens for 13 platforms. For the change: (1) Secrets scan — any API keys, tokens, credentials in code or configs? (2) Verify correctness — trace the logic. (3) Check omissions — missing error handling, unchecked returns. (4) One-pass security/perf scan — obvious exploits, credential leaks, or inefficiencies? **Output:** Verdict (SHIP IT / FIX FIRST), Issues scored 0-100 (only report 75+; if none say "Clean").

Then proceed directly to **Phase 3: Final Verdict**.

### MEDIUM RISK → Dual Review (2 agents, ~60 seconds)

Launch **two parallel Opus agents** plus one Haiku context agent.

**Agent M1 — Security (Cipher):** Use `.claude/commands/review-security.md`. Focus: secrets exposure, auth bypass, vault integrity, HMAC tampering, token leakage.

**Agent M2 — Specialist (pick most relevant):**
- OAuth/auth/vault changes → `.claude/commands/review-veteran.md` (Architect)
- API handlers/HTTP/rate limiting → `.claude/commands/review-performance.md` (Profiler)
- User-facing code/Electron/UX → `.claude/commands/review-ux.md` (Curator)

Both score findings 0-100. Then launch **one Opus adversary agent** (Phase 2) to validate. Proceed to **Phase 3**.

### HIGH RISK → Full Panel Review (7+ agents, ~2 minutes)

Execute the complete panel below.

## Full Panel: Phase 0 Context Gathering

Launch **three parallel Haiku agents**:

**Agent 0A — Change Summary:** Identify ALL files modified/created/deleted. For each: summarize what changed and why. Return structured list: `{ file, changeType, summary }`.

**Agent 0B — Project Standards:** Read `D:\autoCMO-claude\CLAUDE.md` plus any CLAUDE.md in directories containing changed files. Read security architecture section (vault.go patterns, hook security, rate limiting). Return relevant rules/standards.

**Agent 0C — Git History:** For each changed file, run `git log --oneline -10 -- "<filepath>"`. Return recent commits per file, highlighting security-related changes, recent fixes, fragile areas.

Collect all three results before proceeding.

## Full Panel: Phase 1 — Independent Expert Reviews

Launch **four parallel Opus subagents**. Each receives Phase 0 context but operates independently — they MUST NOT see each other's output.

Pass each agent: (1) full conversation context, (2) change summary from 0A, (3) project standards from 0B, (4) git history from 0C.

### Agent 1A — Cipher (Security Expert)
Use `.claude/commands/review-security.md`. Additional: check git history for security hardening that this change might weaken. Return structured findings, each scored 0-100.

### Agent 1B — Architect (Senior Engineer)
Use `.claude/commands/review-veteran.md`. Additional: check if error handling patterns are consistent with existing Go idioms. Return structured findings, each scored 0-100.

### Agent 1C — Profiler (Performance Expert)
Use `.claude/commands/review-performance.md`. Additional: check if rate limiting or HTTP patterns were recently optimized and this change regresses them. Return structured findings, each scored 0-100.

### Agent 1D — Curator (UX Expert)
Use `.claude/commands/review-ux.md`. Additional: check if user-facing messages are clear to a non-technical user. Return structured findings, each scored 0-100.

Collect all four results before proceeding.

## Full Panel: Phase 2 — Adversarial Validation Loop

The adversary stage **repeats until 0 issues >= 75 are found, or 3 adversarial loops have completed**.

### Loop Structure

**Loop N (starting at N=1):**

1. Launch a **single Opus agent** — the Adversary. Pass it: all prior findings + Phase 0 context + (if N>1) fixes applied in prior loops.
2. Adversary returns validated findings scored 0-100.
3. **If any findings score >= 75:** Orchestrator applies fixes, increments N, re-runs adversary on UPDATED code.
4. **If 0 findings score >= 75:** Exit loop — proceed to Build Gate.
5. **If N > 3:** Exit loop — report remaining findings.

### Adversary Prompt

You are the Adversarial Validator for Merlin (AI-powered CMO desktop app — Electron + Go binary). Loop N of max 3. Three responsibilities:

**Responsibility 1 — Challenge Every Finding:** For each: (1) Try to disprove — read actual code, trace execution, check upstream handling. (2) Check anchoring bias — real issue or sounds scary? (3) Check git history — intentional decision? (4) Re-score 0-100: 0-24 false positive, 25-49 nitpick, 50-74 real but low priority, 75-89 fix before shipping, 90-100 critical.

**Responsibility 2 — Gap Analysis:** Start fresh: What did nobody examine? Interaction effects? Assumptions without verification? Unchecked errors?

**Responsibility 3 — Merlin Security Checklist:**
- No API keys, tokens, or credentials in committed files
- No plaintext secrets — vault.go AES-256-GCM for storage, safeStorage in Electron
- OAuth tokens session-scoped only (not persisted unencrypted)
- @@VAULT@@ placeholders used, never raw credentials in merlin-config.json
- WebSocket auth requires token handshake before commands
- User-facing errors are friendly — no stack traces, no raw JSON, no internal paths
- HMAC signatures on rate limit state (tamper → 24h safe mode)
- garble obfuscation on embedded credentials in Go binary
- block-api-bypass.js hook blocks direct platform API calls
- Electron preload.js doesn't expose Node.js APIs to renderer
- No secrets in git history (check with `git log -p --all -S "key" -- "*.go"`)
- CI secrets reference only (never inline values)
- Redirect URI validation in OAuth flows
- Token refresh handles expiry gracefully (no infinite loops)
- Rate limit backoff is exponential (not linear, not infinite)

Score any new issues on the same 0-100 scale.

**Output Format:**
```
ADVERSARY LOOP N/3:

VALIDATED FINDINGS (score >= 75):
1. [CIPHER|ARCHITECT|PROFILER|CURATOR|GAP] Description — Confidence: XX/100 — Evidence: (verified) — File: path (lines X-Y)

DOWNGRADED FINDINGS (score < 75):
1. [SOURCE] Description — Original: XX → Revised: XX — Reason: why not actionable

GAP FINDINGS (new):
1. Description — Confidence: XX/100 — Evidence: (found) — File: path (lines X-Y)

SECURITY CHECKLIST:
- [PASS|FAIL] Each item with status

LOOP RESULT: CLEAN (0 issues >= 75) | FIX REQUIRED (N issues >= 75)
```

### Code Fix Quality Standard

All fixes MUST be:
- **Production-ready** — no "good enough" or "this should work"
- **Security-hardened** — every credential path traced, every auth check verified
- **Go-idiomatic** — proper error wrapping, no swallowed errors, defer for cleanup
- Complete, copy-paste-ready code blocks — not pseudocode

## Phase 2.5: Build Gate

After the adversary loop completes:

1. **Build:** `cd autocmo-core && go build -o Merlin.exe .` — must pass.
2. **Vet:** `go vet ./...` — report warnings.
3. **Test:** `go test ./...` — all tests must pass.
4. **Secrets scan:** `git diff HEAD --cached | grep -iE "(api.?key|secret|token|password|credential)" || echo "clean"` — must be clean.

Build failure = review cannot pass.

## Phase 3: Final Verdict

Using ONLY validated output, compile the final review. **Only include findings scored 75+.**

```
╔══════════════════════════════════════════════════════════════════════════╗
║  PANEL REVIEW — FINAL VERDICT                                         ║
╠══════════════════════════════════════════════════════════════════════════╣
║  Risk Level:     LOW / MEDIUM / HIGH                                   ║
║  Review Tier:    Quick (1) / Dual (2+adversary) / Full Panel (4+adv)  ║
║  Adversary:      N/3 loops — CLEAN / N remaining issues                ║
║  Build Gate:     PASS / FAIL                                           ║
║  Security Gate:  PASS / FAIL                                           ║
║  Secrets Scan:   CLEAN / N findings                                    ║
╠══════════════════════════════════════════════════════════════════════════╣

FINDINGS (N issues, scored 75+)
1. [SEVERITY] [SOURCE] Description
   - Confidence: XX/100 | File: path (lines X-Y)
   - Evidence: ... | Fix: ...

SECURITY FINDINGS (always reported separately)
1. [CRITICAL|HIGH|MEDIUM] Description
   - Attack vector: ...
   - Impact: ...
   - Fix: ...

DISMISSED (M findings scored < 75)
- Brief summary and why rejected

PANEL CONSENSUS
- Issues flagged by 2+ reviewers

SECURITY CHECKLIST
- [PASS/FAIL] Each Merlin security item

VERDICT: Ship It? YES / YES WITH CHANGES / NO
Top Actions (ranked): 1. ... 2. ... 3. ...

╚══════════════════════════════════════════════════════════════════════════╝
```

## Execution Notes

- Use `model: opus` for ALL subagents (reviewers + adversary), `model: haiku` for Phase 0 context gatherers only
- Phase 1 agents MUST run in parallel (independent, no cross-contamination)
- Phase 2 MUST wait for all Phase 1 agents to complete
- Build Gate runs AFTER adversary loop, BEFORE final verdict
- **Security findings are ALWAYS reported** — even if scored below 75, security issues get a separate section
- Every review includes secrets scanning regardless of risk level
- If any `git` commands fail, continue without history
- If no findings score 75+, report "No actionable issues found" with brief summary of what was checked
