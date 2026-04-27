// Source-scan regression test for the silent-refresh fix (2026-04-27).
//
// Background: the Claude Agent SDK refreshes its OAuth access token
// silently against platform.claude.com/v1/oauth/token using the
// refreshToken stored in ~/.claude/.credentials.json. Two preconditions
// are required for that to work:
//
//   (1) extractToken() must NOT pre-reject expired tokens — the SDK is
//       the entity that decides whether to refresh, and it does so when
//       it reads the file directly.
//
//   (2) startSession() must NOT inject CLAUDE_CODE_OAUTH_TOKEN env when
//       the file has refresh capability. The SDK's env-fed code path
//       hard-codes refreshToken=null, so env injection silently disables
//       refresh and forces a full browser sign-in on every TTL boundary.
//
// Live incident 2026-04-27 was the symptom of (2) (and the 5-minute
// pre-rejection in (1) compounded it). Both regressions are silent-
// failure-mode — nothing throws, nothing logs, paying users just get
// kicked into a browser flow they shouldn't see.
//
// Run with: node app/sdk-token-silent-refresh.test.js

const fs = require('fs');
const path = require('path');

const APP_DIR = __dirname;
const MAIN_JS = path.join(APP_DIR, 'main.js');
const HELPER_JS = path.join(APP_DIR, 'auth-credentials.js');

let passed = 0;
let failed = 0;

function test(name, fn) {
  try {
    fn();
    console.log('  ✓', name);
    passed++;
  } catch (err) {
    console.log('  ✗', name);
    console.log('   ', err.message);
    failed++;
  }
}

for (const [label, p] of [['main.js', MAIN_JS], ['auth-credentials.js', HELPER_JS]]) {
  if (!fs.existsSync(p)) {
    console.error(`FATAL: ${label} not found at ${p}`);
    process.exit(1);
  }
}
const MAIN = fs.readFileSync(MAIN_JS, 'utf8');
const HELPER = fs.readFileSync(HELPER_JS, 'utf8');

// ── Helper module is required, not duplicated ────────────────────────────

test('main.js requires extractToken + readFileCredentialsForSession from auth-credentials', () => {
  if (!/require\(\s*['"]\.\/auth-credentials['"]\s*\)/.test(MAIN)) {
    throw new Error(
      'main.js no longer requires `./auth-credentials`. The credential-'
      + 'parsing helpers MUST live in that module so they remain testable '
      + 'without booting Electron. Restore the require.'
    );
  }
  if (!/extractToken\b/.test(MAIN) || !/readFileCredentialsForSession/.test(MAIN)) {
    throw new Error(
      'main.js no longer references both extractToken AND '
      + 'readFileCredentialsForSession by name. Both helpers gate the '
      + 'silent-refresh behavior — neither can be inlined or removed.'
    );
  }
});

test('main.js does NOT redefine extractToken inline (would shadow helper)', () => {
  // Look for `function extractToken(` in main.js. The require destructure
  // is fine, but a `function extractToken(...)` declaration would shadow
  // the helper and silently lose any future fix in auth-credentials.js.
  if (/function\s+extractToken\s*\(/.test(MAIN)) {
    throw new Error(
      'main.js redefines `function extractToken(...)` inline. This '
      + 'shadows the imported helper from auth-credentials.js. Remove '
      + 'the inline definition.'
    );
  }
});

// ── Helper enforces the contract ─────────────────────────────────────────

test('auth-credentials.js extractToken does NOT auto-reject expired tokens', () => {
  // Strip line comments and block comments so a documentary comment that
  // mentions the historical "expired" behavior doesn't trigger a false
  // positive against the live code.
  const code = HELPER
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '');
  // Look for any combination of "expired" or `Date.now() > ` followed by
  // an early `return null` — the exact shape of the pre-fix rejection.
  if (/console\.log\(\s*['"`]\[auth\]\s*Token expired,\s*skipping['"`]\s*\)/.test(code)) {
    throw new Error(
      'auth-credentials.js extractToken re-introduces the "[auth] Token '
      + 'expired, skipping" early-return. That rejection turned every '
      + 'expiry into a forced browser sign-in even when the SDK could '
      + 'have refreshed silently. Remove it.'
    );
  }
  if (/Date\.now\(\)\s*>\s*expiresMs\s*\+\s*\d+\s*\)\s*\{[^}]*return\s+null/.test(code)) {
    throw new Error(
      'auth-credentials.js extractToken contains an early-return on '
      + 'expiry. Tokens past expiresAt MUST still surface to the caller '
      + 'so the SDK can decide whether to refresh.'
    );
  }
});

test('auth-credentials.js extractToken surfaces refreshable + expiresAt', () => {
  if (!/refreshable/.test(HELPER)) {
    throw new Error(
      'auth-credentials.js no longer surfaces a `refreshable` flag. '
      + 'startSession needs that flag to decide whether to inject env. '
      + 'Restore it.'
    );
  }
  if (!/expiresAt/.test(HELPER)) {
    throw new Error(
      'auth-credentials.js no longer surfaces `expiresAt`. Without it '
      + 'callers can\'t detect proactive-refresh windows.'
    );
  }
});

// ── startSession honors the Tier 1 / Tier 2 split ────────────────────────

test('main.js startSession Tier 1 path skips env injection on refreshable file', () => {
  // The Tier 1 branch must call readFileCredentialsForSession() and, on
  // a hit, NOT set sessionEnv.CLAUDE_CODE_OAUTH_TOKEN. The simplest
  // enforceable shape: inside the readFileCredentialsForSession()-truthy
  // branch, there should be a `console.log` mentioning skipping env
  // injection. Source-scan only — full behavioral coverage lives in
  // auth-credentials.test.js.
  if (!/readFileCredentialsForSession\s*\(/.test(MAIN)) {
    throw new Error(
      'main.js no longer calls readFileCredentialsForSession() in '
      + 'startSession. That call is the entry point of the Tier 1 path '
      + 'and the load-bearing piece of silent refresh. Restore it.'
    );
  }
  // The phrase "skipping env injection" appears exclusively in the Tier 1
  // success log line. If a future edit removes the log, the inline test
  // body comment ("we deliberately do NOT set sessionEnv...") becomes
  // unreachable from grep — flag it.
  if (!/skipping env injection/.test(MAIN)) {
    throw new Error(
      'main.js Tier 1 success path no longer logs "skipping env '
      + 'injection". The log line is the canary that the env-skip is '
      + 'still happening. Restore it.'
    );
  }
});

test('main.js Tier 1 + Tier 2 are an if/else (no fall-through)', () => {
  // The branching pattern: `if (fileCreds) { … } else { … readCredentials }`.
  // If a future edit collapses Tier 1 into a non-conditional, env-injection
  // could fire on top of the file path and silently re-disable refresh.
  // Match the exact shape we shipped: `const fileCreds = readFileCredentialsForSession();`
  // followed within ~1500 chars by `} else {`.
  const m = MAIN.match(/const\s+fileCreds\s*=\s*readFileCredentialsForSession\s*\([\s\S]{0,1500}else\s*\{/);
  if (!m) {
    throw new Error(
      'main.js startSession no longer branches with `if (fileCreds) {…} '
      + 'else {…readCredentials…}`. Without the else, both paths run and '
      + 'env-injection re-disables refresh. Restore the if/else.'
    );
  }
});

test('main.js keeps the REGRESSION GUARD (2026-04-27, sdk-token-silent-refresh) block', () => {
  if (!MAIN.includes('REGRESSION GUARD (2026-04-27, sdk-token-silent-refresh)')) {
    throw new Error(
      'main.js lost the REGRESSION GUARD (2026-04-27, sdk-token-silent-'
      + 'refresh) comment block. That block is the human-readable record '
      + 'of the live incident — DO NOT delete it. If the rule changed, '
      + 'add a new dated block above explaining what changed.'
    );
  }
});

test('auth-credentials.js keeps the REGRESSION GUARD (2026-04-27, sdk-token-silent-refresh) block', () => {
  if (!HELPER.includes('REGRESSION GUARD (2026-04-27, sdk-token-silent-refresh)')) {
    throw new Error(
      'auth-credentials.js lost the REGRESSION GUARD comment. The '
      + 'reasoning for the no-expiry-rejection rule MUST stay co-located '
      + 'with the code so a future maintainer doesn\'t "fix" it.'
    );
  }
});

// ─── done ──────────────────────────────────────────────────────────────────
console.log('');
console.log(`  ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
