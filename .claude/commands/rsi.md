---
description: "Recursive Self-Improvement — iterative audit-fix-verify loops targeting 10/10 for security, UX, and reliability"
allowed-tools: Agent, Bash, Read, Edit, Write, Glob, Grep, TodoWrite
---

# /rsi — Recursive Self-Improvement Engine

**Purpose**: Systematically audit, score, fix, and re-score any Merlin subsystem through iterative improvement loops until it reaches a 10/10 reliability score. What makes this *recursive* is that each iteration refines not just the target, but the improvement process itself — better heuristics, sharper audits, and locked-in gains that can't regress.

The core insight: **an improvement that can regress isn't an improvement. An audit that misses the same class of bug twice isn't learning. A fix that treats symptoms instead of causes will generate more work than it saves.**

**The Merlin standard**: Ship complete, correct, autonomous systems at pace. 10/10 means "a non-technical business owner can use this feature without confusion, without data loss, and without security exposure." Not "the known bug list is empty" — that's weaker. The test is: *could a 5th grader use this correctly, and could a pentester not break it?*

**RSI focus for Merlin — two lenses on every target:**
1. **Security**: Secrets management, encryption, OAuth integrity, vault operations, HMAC verification, rate limiting tamper resistance, credential lifecycle, transport security, CI/CD secret handling
2. **UX**: Apple-tier simplicity, plain English errors, zero-jargon user flows, one-click OAuth, clear progress feedback, actionable failure recovery, 5th-grader test on every user-facing element

## Arguments

`$ARGUMENTS` — parse for subcommand or improvement target:

| Input | Action |
|-------|--------|
| A target description (default) | Start new RSI session |
| `resume` | Resume from checkpoint file |
| `status` | Show current iteration, score, remaining issues, improvement velocity |
| `stop` | Halt after current iteration, present partial report |
| `score` | Re-display the most recent scorecard |
| `meta` | Show meta-learning: what the process learned about itself |

**Target examples:**
- `/rsi OAuth flow — goal: every platform connects in under 60 seconds with zero jargon`
- `/rsi vault.go credential security — goal: zero plaintext secrets, zero credential leaks`
- `/rsi error messages — goal: every error is actionable plain English, 5th-grader readable`
- `/rsi Meta Ads integration — full security + UX audit`
- `/rsi Electron preload security — goal: zero Node.js API exposure to renderer`
- `/rsi onboarding flow — goal: first valuable result within 5 minutes`
- `/rsi rate limiting — goal: zero bypass vectors, zero HMAC tampering paths`

If no argument provided, ask the user what system they want to improve.

### Goal Extraction
If the argument contains a measurable goal (e.g., "zero plaintext secrets", "under 60 seconds", "5th-grader readable"), extract it as the **convergence criterion** — the loop continues until both the score reaches 10/10 AND the goal is met.

---

## THE CARDINAL RULE: Rubric Before Plan

**The rubric defines 10/10. The plan is how to get there. These are NEVER the same document.**

The catastrophic failure mode: you build a plan, execute it, audit its execution, and declare victory. This is circular. The plan was incomplete, so auditing its completion proves nothing.

The fix: **define what 10/10 looks like BEFORE knowing what work needs to be done.** The rubric is permanent. The plan is disposable.

---

## Phase 0: Understand & Scope (MANDATORY)

### 0a. Context Gathering
1. Read conversation history — has the user identified problems or a target?
2. Read `D:\autoCMO-claude\CLAUDE.md` for architecture context
3. Read relevant source files in `autocmo-core/` or `autoCMO/app/`
4. Check for prior RSI sessions — read any checkpoint files in project
5. If target is Go source: identify key files, entry points, test files (`*_test.go`)
6. If target is Electron: identify main.js, renderer.js, preload.js interaction
7. If target is Workers: identify worker.js, wrangler.toml, KV bindings

### 0b. Define the End State (BEFORE proposing work)

**This is the most important step.** Before looking at what's broken, define what "done" looks like.

Write a **Definition of Done (DoD)** as testable properties. Each must be:
- **Testable**: yes/no verification, not "good" or "adequate"
- **Independent of implementation**: describes WHAT, not HOW
- **Necessary**: removing it would leave the system incomplete
- **Sufficient together**: ALL items passing = genuinely 10/10

**DoD template:**
```
DEFINITION OF DONE — [Target Name]
A non-technical business owner can use this feature, and a security auditor cannot break it, when:

SECURITY:
□ [No plaintext credentials anywhere in codebase or git history]
□ [All API tokens encrypted at rest with AES-256-GCM]
□ [OAuth state parameter validated on every callback]
□ ...

UX:
□ [Every error message is actionable plain English — no codes, no jargon]
□ [User completes the flow without reading documentation]
□ [Money-related actions show exact amounts before confirmation]
□ ...

RELIABILITY:
□ [All error paths return gracefully — no panics, no swallowed errors]
□ [Operation is idempotent — safe to retry on failure]
□ ...

Any item unchecked = NOT 10/10.
```

**Good DoD items for Merlin:**
- "No OAuth token is stored outside the vault or Electron safeStorage"
- "Every user-facing error message passes the 5th-grader test"
- "Rate limit state cannot be tampered without triggering safe mode"
- "Go binary compiles with zero `go vet` warnings"
- "Every API call has a timeout and handles context cancellation"
- "User never sees a raw API error, JSON payload, or stack trace"

**BAD DoD items:**
- "The security is robust" (undefined)
- "Fix vault.go to use AES-256" (task, not property)
- "Add better error handling" (compared to what?)
- "Make the UX good" (unmeasurable)

Present DoD to user for approval. **Do not proceed until confirmed.** Once approved, the DoD is frozen.

### 0c. Define the Scoring Rubric (Derived from DoD)

Group DoD items into 4-8 scoring categories. For Merlin, always include:

```
╔══════════════════════════════════════════════════════════════════════════╗
║  SCORING RUBRIC — [Target Name]                                       ║
╠══════════════════════════════════════════════════════════════════════════╣
║                                                                        ║
║  Each category scored 0-10. Final score = lowest category score.       ║
║  (A chain is only as strong as its weakest link.)                      ║
║                                                                        ║
║  1. Credential Security — DoD items: #...             ___/10           ║
║     10 = zero plaintext secrets, vault encryption verified             ║
║      5 = some secrets encrypted, gaps remain                           ║
║      0 = plaintext credentials in code or config                       ║
║                                                                        ║
║  2. Auth & OAuth Integrity — DoD items: #...          ___/10           ║
║     10 = all 13 providers secure, PKCE/state validated                 ║
║      5 = most providers secure, edge cases unhandled                   ║
║      0 = auth bypass possible                                          ║
║                                                                        ║
║  3. User Experience — DoD items: #...                 ___/10           ║
║     10 = 5th-grader passes every flow, Apple-tier polish               ║
║      5 = functional but confusing in places                            ║
║      0 = users get stuck or see technical errors                       ║
║                                                                        ║
║  4. Error Handling & Recovery — DoD items: #...       ___/10           ║
║     10 = every error actionable, every failure recoverable             ║
║      5 = most errors handled, some silent failures                     ║
║      0 = panics, swallowed errors, or unrecoverable states             ║
║                                                                        ║
║  [Additional categories as needed for target]                          ║
║                                                                        ║
║  OVERALL: ___/10 (= min of all categories)                             ║
╚══════════════════════════════════════════════════════════════════════════╝
```

### 0c-ii. Bind Objective Metrics (MANDATORY)

At least **one rubric category** must have a machine-verifiable metric:

**Merlin metric bindings (use where applicable):**

| Category | Command | Scoring |
|----------|---------|---------|
| Build Health | `cd autocmo-core && go build -o /dev/null . 2>&1 | grep -c "error"` | 10=0, 5=1-3, 0=4+ |
| Vet Clean | `cd autocmo-core && go vet ./... 2>&1 | grep -c ":"` | 10=0, 8=1-3, 5=4-10, 0=11+ |
| Test Pass | `cd autocmo-core && go test ./... 2>&1 | grep -c "FAIL"` | 10=0, 5=1-2, 0=3+ |
| Secrets Scan | `grep -rn "sk-\|AKIA\|ghp_\|xox[bpas]-" autocmo-core/*.go | grep -cv "test\|example"` | 10=0, 0=1+ |
| Plaintext Check | `grep -rn "@@VAULT" autocmo-core/*.go | wc -l` (count should match expected) | context-dependent |

Rules:
- Metric commands run at baseline (Phase 2) and every re-score (Phase 3e)
- Metric score and self-assessed score disagree by 2+ → metric wins
- Convergence requires ALL metric-bound categories at 10/10 via actual output

### 0d. Specify Invariants

Properties that must ALWAYS hold:

**Merlin security invariants:**
- No credential value appears in any committed file (git grep)
- @@VAULT@@ placeholder count matches expected encrypted values
- OAuth redirect URI is always `https://merlingotme.com/auth/callback`
- Rate limit HMAC uses crypto/hmac, not manual hash comparison
- Vault key derivation uses hostname+username, never hardcoded
- garble build flags present in CI for production binaries
- Electron contextIsolation=true, nodeIntegration=false
- WebSocket requires auth handshake before accepting commands

**Merlin UX invariants:**
- No error message contains: "Error:", "Exception", stack traces, JSON, or API codes
- Every money-related action shows the amount before confirmation
- Every OAuth flow completes with a single user click after browser opens
- Every loading state shows progress for operations > 2 seconds
- Every empty state explains what to do next

Write invariants as machine-checkable assertions where possible.

### 0e. Propose Improvements and Build the Plan

NOW — after DoD and rubric are locked — analyze current state:

- **What exists today** — brief capability summary
- **What's missing vs. DoD** — gap analysis mapped to specific items
- **Proposed work** — concrete tasks by rubric category
- **Recommended scope** — this session vs. defer

Wait for user confirmation. The plan is a hypothesis — if executing it doesn't achieve 10/10, the plan was wrong.

---

## Phase 1: Deep Parallel Audit

Launch **parallel Opus audit agents** covering the target:

- **Small target** (1-3 files): 2-3 agents (security lens, UX lens, reliability lens)
- **Medium target** (4-10 files): 4-6 agents by subsystem
- **Large target** (10+ files): 6-10 agents by component

### Each audit agent MUST receive:
1. The **full DoD**
2. The **rubric categories** they're auditing
3. The **invariants** from Phase 0d
4. Instruction: "Score against the DoD, not any fix plan."

### Each audit agent MUST:
1. Read every relevant file exhaustively
2. Check all invariants against their subsystem
3. Check every DoD item: PASS / PARTIAL / FAIL with evidence
4. Categorize: **Critical** (security breach/data loss), **High** (silent failure/UX wall), **Medium** (degraded experience), **Low** (polish)
5. Confidence: P(real issue) — 50% = suspicious, 90% = demonstrated
6. File paths and line numbers for every finding
7. Concrete evidence, not vague concerns
8. Root causes vs. symptoms — if 3 issues stem from 1 flaw, report the root

### Audit agent prompt template:
```
Audit [SUBSYSTEM] against the Definition of Done for Merlin. Read every file.

DEFINITION OF DONE:
□ [items...]

Your rubric categories:
- [Category]: DoD items #X, #Y

Files to audit:
- [files...]

Invariants to verify:
- [invariants...]

SECURITY LENS: Check for credential exposure, auth bypass, encryption gaps,
HMAC tampering, token leakage, injection vectors, privilege escalation.

UX LENS: Check for jargon in errors, unclear flows, missing progress indicators,
confusing terminology, technical leakage to users, broken recovery paths.

For EACH DoD item: PASS / PARTIAL / FAIL with evidence.
Return: DoD scorecard + issue list grouped by root cause.
```

Wait for ALL agents to complete.

### 1b. Build Causal Graph

```
Root Cause A (e.g., "raw API errors bubble to user")
├── Symptom A1: Meta OAuth error shows "OAuthException" (Critical/UX)
├── Symptom A2: Shopify rate limit shows "429 Too Many Requests" (High/UX)
└── Symptom A3: Google Ads timeout shows "context deadline exceeded" (High/UX)

Root Cause B (e.g., "token stored outside vault")
└── Symptom B1: Refresh token in plaintext merlin-config.json (Critical/Security)
```

Fix order: root causes first → eliminates downstream symptoms.

---

## Phase 2: Score Baseline

Score each category 0-10 **against DoD items, not tasks.**

```
╔══════════════════════════════════════════════════════════════════════════╗
║  BASELINE SCORE — Iteration 0                                         ║
╠══════════════════════════════════════════════════════════════════════════╣
║                                                                        ║
║  DoD SCORECARD:                                                        ║
║  □ Item 1: [PASS/PARTIAL/FAIL] — [evidence]                          ║
║  ...                                                                   ║
║                                                                        ║
║  RUBRIC:                                                               ║
║  Credential Security:     X/10 — N/M DoD items passing                ║
║  Auth & OAuth Integrity:  X/10                                         ║
║  User Experience:         X/10                                         ║
║  Error Handling:          X/10                                         ║
║  OVERALL: X/10                                                         ║
║                                                                        ║
║  Root causes: N | Symptoms: N | DoD passing: N/M                      ║
╚══════════════════════════════════════════════════════════════════════════╝
```

---

## Phase 3: Iterative Fix Loops

**Minimum 3 iterations. Maximum 10 per approval cycle. Continue until 10/10. No "good enough."**

Acceptable stops: (1) 10/10 achieved, (2) iteration 10 → checkpoint + human approval, (3) user sends `/rsi stop`, (4) structural blocker needs user decision.

### 3a. Prioritize (Root-Cause-First)
1. **Root causes** eliminating the most symptoms
2. **Security critical** (credential exposure, auth bypass)
3. **UX critical** (users get stuck, abandon flow)
4. **Dependency chain** blockers
5. **Quick wins** flipping DoD items to PASS

### 3b. Fix
For Go changes:
- Edit minimal code
- Re-read modified files to verify
- Trace happy path + failure path
- Check error wrapping (`fmt.Errorf("...: %w", err)`)
- Verify no credential leakage in new code

For Electron/Worker changes:
- Check user-facing text passes 5th-grader test
- Verify no technical jargon leaks
- Test error recovery paths

### 3c. Lock In Gains (what makes this recursive)

After each fix, **prevent regression**:

1. **Go test** — write `*_test.go` regression test. Best lock.
2. **Validation check** — add to vet/lint/build verification
3. **Runtime guard** — assertion or early return that catches the pattern
4. **Documented invariant** — add to Phase 0d invariants list

Hierarchy: **test > build check > runtime guard > documented invariant.**

**Security-specific locks:**
- Secrets scan in CI (grep for patterns)
- Vault placeholder count assertion
- OAuth state parameter test
- HMAC verification test

**UX-specific locks:**
- Error message format test (no jargon patterns)
- User-facing string review checklist

### 3d. Verify
- `go build -o /dev/null .` — must compile
- `go vet ./...` — clean
- `go test ./...` — all pass (including NEW tests from 3c)
- Secrets scan — clean
- Manual spot-checks on changed areas

**If a fix makes score worse:** Revert, log as fix regression, update causal model, try alternative.

### 3e. Re-Score (Independent Verification — MANDATORY)

**Track 1 — Metric-bound:** Run all CLI commands from 0c-ii. Objective, final.

**Track 2 — Non-metric:** Launch **independent Opus scoring agent** that:
1. Receives DoD and rubric (NOT fix plan or task list)
2. Receives files modified this iteration
3. Receives previous DoD scorecard
4. Does NOT see what was fixed
5. Reads current code fresh
6. Re-evaluates every DoD item: PASS / PARTIAL / FAIL
7. Returns scores with justification

```
╔══════════════════════════════════════════════════════════════════════════╗
║  ITERATION N SCORE                                                     ║
╠══════════════════════════════════════════════════════════════════════════╣
║  DoD: Item 1: FAIL → PASS                                             ║
║       Item 2: PARTIAL → PARTIAL (still missing: X)                     ║
║  Passing: N/M (was K/M)                                                ║
║                                                                        ║
║  RUBRIC:                                                               ║
║  Credential Security:  X → Y/10  [METRIC: output]                     ║
║  User Experience:      X → Y/10  [INDEPENDENT SCORER]                  ║
║  OVERALL: X → Y/10                                                     ║
║                                                                        ║
║  Fixed: [list] | Emergent issues: [count] | Locks added: [count]      ║
║  Velocity: [DoD items flipped to PASS / iteration]                     ║
╚══════════════════════════════════════════════════════════════════════════╝
```

### 3f. Meta-Learning Checkpoint (every 2 iterations)

Reflect on the PROCESS:
1. **Audit completeness**: Did initial audit catch everything, or is scorer finding new gaps?
2. **Plan accuracy**: How many tasks actually moved DoD items to PASS?
3. **Fix quality**: Are fixes sticking (locked by tests) or regressing?
4. **Scoring calibration**: Is the scorer consistent and evidence-based?
5. **Pattern recognition**: Same class of gap repeating? → define heuristic rule

### 3g. Loop Decision

**Decision tree:**
1. **ALL DoD items PASS AND 10/10** → Phase 4. Victory.
2. **Iteration = 10 AND < 10/10** → MANDATORY STOP. Present checkpoint:

```
╔══════════════════════════════════════════════════════════════════════════╗
║  ITERATION CAP — Human Approval Required                              ║
╠══════════════════════════════════════════════════════════════════════════╣
║  Score: X/10 (started Y/10) | DoD: N/M passing                       ║
║  Velocity (last 3): [trend]                                            ║
║                                                                        ║
║  FAILING DoD Items:                                                    ║
║  □ Item N: FAIL — [why, what's hard]                                  ║
║                                                                        ║
║  OPTIONS:                                                               ║
║  1. Continue 10 more (same approach)                                   ║
║  2. Continue with revised approach                                     ║
║  3. Revise DoD                                                         ║
║  4. Accept current score → Phase 4                                     ║
║  5. Stop                                                                ║
╚══════════════════════════════════════════════════════════════════════════╝
```

3. **User sends `/rsi stop`** → Finish current fix, verify build, present partial report.
4. **Blocked** → Flag, skip to other fixes. Only stop if ALL remaining issues blocked.
5. **Score < 10/10 AND iterations remain** → Continue.
6. **Velocity plateau** (2 iterations, no DoD flips) → Change approach, don't stop.

---

## Phase 3h: Context Window Management

### Checkpoint Protocol (at 80% context OR iteration 10)

Write checkpoint capturing full RSI state:

```
# RSI Checkpoint — [Target Name]
## Resume with `/rsi resume`

### State
- Target: [description]
- Iteration: N of 10 (cycle M)
- Score: X/10 (started Y/10)

### Definition of Done (FROZEN)
□ [items with PASS/PARTIAL/FAIL status]

### Rubric + Metric Bindings
[scores and commands]

### Invariants
[all invariants]

### Causal Graph
[root causes and status]

### Remaining Issues + Regression Locks + Files Modified
[details]

### Meta-Learning + Next Steps
[heuristics learned, what to prioritize next]
```

### Resume Protocol
When `/rsi resume`:
1. Read checkpoint file
2. Reconstruct DoD, rubric, invariants, causal graph
3. Run all metric commands to verify scores haven't drifted
4. Re-run independent scorer on ALL DoD items
5. If scores match: continue. If diverged: re-score completely.

---

## Phase 4: Final Verification & Report

### 4a. Full DoD Sweep
Launch **independent Opus agent** evaluating EVERY DoD item from scratch — no memory of prior scores. This is the definitive 10/10 gate. Any failure → return to Phase 3.

### 4b. Full Regression
- `go build` — clean
- `go vet` — clean
- `go test ./...` — all pass
- Secrets scan — clean
- All new tests pass

### 4c. Adversarial Spot-Check
Launch **Opus adversarial agent**:
1. Pick 3-5 operations, trace end-to-end
2. Try to BREAK the system — stolen tokens, forged HMACs, jargon injection, XSS in error messages
3. Verify regression locks actually catch their target bugs
4. Verify all invariants still hold

### 4d. Final Report

```
╔══════════════════════════════════════════════════════════════════════════╗
║  RSI FINAL REPORT — [Target Name]                                     ║
╠══════════════════════════════════════════════════════════════════════════╣
║                                                                        ║
║  Starting Score:  X/10                                                 ║
║  Final Score:     10/10 (verified independently)                       ║
║  Iterations:      N                                                    ║
║                                                                        ║
║  DoD: M/M PASS                                                        ║
║  □ Item 1: PASS                                                       ║
║  □ Item 2: PASS                                                       ║
║  ...                                                                   ║
║                                                                        ║
║  SECURITY SUMMARY:                                                     ║
║  - Credentials: [vault-encrypted, zero plaintext]                     ║
║  - OAuth: [all providers verified, PKCE/state validated]              ║
║  - Transport: [TLS everywhere, HMAC verified]                         ║
║  - Secrets scan: CLEAN                                                 ║
║                                                                        ║
║  UX SUMMARY:                                                           ║
║  - Error messages: [all plain English, actionable]                    ║
║  - User flows: [5th-grader tested, Apple-tier]                        ║
║  - Empty/loading states: [all handled]                                ║
║                                                                        ║
║  Issues Found: N (N critical, N high, N medium, N low)                ║
║  Root Causes: N identified                                             ║
║  Fixed: N | Emergent: N | Deferred: N                                 ║
║                                                                        ║
║  Regression Locks: N tests, N validators, N guards                    ║
║  Adversarial: [spot checks VERIFIED]                                  ║
║  Invariants: N defined, N machine-checked                              ║
║                                                                        ║
╚══════════════════════════════════════════════════════════════════════════╝
```

---

## Rules

1. **The DoD is the source of truth.** Not the plan, not tasks, not self-assessment.
2. **No hallucinations.** Every finding references a real file:line.
3. **No unverified results.** Every fix verified with build + test.
4. **Root causes first.** Don't patch symptoms.
5. **Lock every gain.** Test > validator > guard > invariant.
6. **Opus for audit and scoring.** Quality determines everything.
7. **Build after every change.** `go build` must pass.
8. **Score = min of all categories.** One weak link = system unreliable.
9. **Scorers never see the fix plan.** DoD + current code only.
10. **Emergent gaps are real.** Scorer finds new issues → add to tracker.
11. **Atomic iterations.** Never stop mid-fix.
12. **Track changes.** Running changelog for accurate final report.
13. **10/10 or bust, 10 at a time.** No voluntary stop below 10/10.
14. **Revert on regression.** Fix makes score worse → undo, rethink.
15. **The process improves too.** Better heuristics each session.
16. **Security and UX are equal.** A secure feature nobody can use is as broken as an easy feature that leaks credentials.
