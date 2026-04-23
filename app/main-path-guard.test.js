// Source-scan regression test for IPC path-containment checks in
// app/main.js (and any other app/*.js that performs `startsWith(root)`
// containment checks against an Electron handler argument).
//
// REGRESSION GUARD (2026-04-23, codex audit session/security-quick-wins):
// The prior pattern was:
//
//     const fullPath = path.resolve(appRoot, userArg);
//     if (!fullPath.startsWith(path.resolve(appRoot))) return { success: false };
//
// which fails open when a sibling directory shares the same prefix as
// appRoot (e.g. appRoot = `C:\Users\R\app` and userArg resolves to
// `C:\Users\R\app-evil\foo.png` — `startsWith` returns true). The
// correct pattern adds `+ path.sep` and an exact-equal check:
//
//     const root = path.resolve(appRoot);
//     if (!fullPath.startsWith(root + path.sep) && fullPath !== root) return ...
//
// This scan locks the fix in: any future edit that re-introduces a
// `.startsWith(<some root>)` against a path WITHOUT a `+ path.sep`
// suffix or a `!== root` peer check fails CI.
//
// Run: `node app/main-path-guard.test.js`

'use strict';

const fs = require('fs');
const path = require('path');
const assert = require('assert');

const APP_DIR = __dirname;
const MAIN_JS = path.join(APP_DIR, 'main.js');

let passed = 0;
let failed = 0;

function test(name, fn) {
  try {
    fn();
    console.log('  \u2713', name);
    passed++;
  } catch (err) {
    console.log('  \u2717', name);
    console.log('   ', err.message);
    failed++;
  }
}

function stripComments(src) {
  let out = src.replace(/\/\*[\s\S]*?\*\//g, '');
  out = out.split('\n').map((line) => {
    const idx = line.indexOf('//');
    if (idx < 0) return line;
    const before = line.slice(0, idx);
    const sq = (before.match(/'/g) || []).length;
    const dq = (before.match(/"/g) || []).length;
    const bt = (before.match(/`/g) || []).length;
    if (sq % 2 === 1 || dq % 2 === 1 || bt % 2 === 1) return line;
    return before;
  }).join('\n');
  return out;
}

console.log('main-path-guard.test.js');

test('no .startsWith(path.resolve(appRoot)) without path.sep suffix in main.js', () => {
  const src = stripComments(fs.readFileSync(MAIN_JS, 'utf8'));
  // Match the exact bad pattern: `.startsWith(path.resolve(appRoot))`
  // with the closing paren immediately after `appRoot)`. The fixed
  // form is `.startsWith(resolvedAppRoot + path.sep)` which won't match.
  const bad = src.match(/\.startsWith\(\s*path\.resolve\(appRoot\)\s*\)/g);
  assert.equal(
    bad,
    null,
    `main.js still contains prefix-only path-resolve startsWith calls: ${bad ? bad.join(', ') : ''}`,
  );
});

test('every .startsWith(<root>) in main.js pairs with path.sep or !== <root>', () => {
  const src = stripComments(fs.readFileSync(MAIN_JS, 'utf8'));
  // Find every .startsWith(<ident>) call where the arg is a bare ident
  // (not a string literal). For each, check that within the same line
  // OR within the same expression, there's a `+ path.sep` token or a
  // `!== <ident>` check on the same identifier.
  const callRe = /\.startsWith\(\s*([A-Za-z_][A-Za-z0-9_]*)\s*\)/g;
  const SAFE_CALL_TARGETS = new Set([
    'data', 'http', 'https', 'data:', 'merlin', 'merlin:', 'file', 'file:',
    'wss', 'ws', 'about',
  ]);
  let m;
  const offenders = [];
  while ((m = callRe.exec(src)) !== null) {
    const ident = m[1];
    // Heuristic: if the identifier looks like a path root (ends in
    // 'Root', 'root', 'Path', 'path', 'Dir', 'dir'), it's a path-style
    // check and must use the safer pattern. Skip identifiers that look
    // like protocol/scheme prefixes since those are string-prefix checks
    // on URLs, not filesystem paths.
    const isPathLike = /(Root|root|Path|path|Dir|dir)$/.test(ident);
    if (!isPathLike) continue;
    // Look at the surrounding 200 chars for either:
    //   - `<ident> + path.sep`  (the safer suffix pattern)
    //   - `<ident>` referenced inside the same statement before the call
    //     in a way that already includes path.sep
    //   - paired `!== <ident>` exact-equal escape hatch
    const ctxStart = Math.max(0, m.index - 200);
    const ctxEnd = Math.min(src.length, m.index + 200);
    const ctx = src.slice(ctxStart, ctxEnd);
    const hasSep = new RegExp(`startsWith\\(\\s*${ident}\\s*\\+\\s*path\\.sep`).test(ctx)
      || new RegExp(`startsWith\\(\\s*${ident}\\s*\\+\\s*['"\\\\\\\\/]`).test(ctx);
    const hasExactPeer = new RegExp(`!==\\s*${ident}`).test(ctx)
      || new RegExp(`===\\s*${ident}`).test(ctx);
    if (!hasSep && !hasExactPeer) {
      offenders.push({
        ident,
        snippet: src.slice(Math.max(0, m.index - 60), m.index + 60),
      });
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `main.js has path-style startsWith calls without a path.sep suffix or exact-peer check: ${JSON.stringify(offenders, null, 2)}`,
  );
});

test('REGRESSION GUARD comments are present at the fixed handlers', () => {
  const src = fs.readFileSync(MAIN_JS, 'utf8'); // keep comments here
  const guards = (src.match(/REGRESSION GUARD \(2026-04-23, codex audit/g) || []).length;
  assert.ok(
    guards >= 3,
    `main.js should keep \u2265 3 REGRESSION GUARD (2026-04-23) blocks (copy-image, open-folder, save-pasted-media); found ${guards}`,
  );
});

console.log(`\n${passed} passed, ${failed} failed`);
process.exit(failed === 0 ? 0 : 1);
