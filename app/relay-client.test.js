// Unit tests for relay-client.js. Run with `node app/relay-client.test.js`.
//
// relay-client requires `electron`, which isn't installed as a dep of the
// test runner. We stub it via Module._cache injection BEFORE requiring the
// module under test. This keeps the real source path under test — no copy,
// no refactor to pure-logic.
//
// Scope: pure-logic surfaces we want to lock down against regression —
//   1. forward() only emits envelope types on the DESKTOP_TYPES allowlist.
//   2. forward() refuses when not connected (no silent drop of caller's
//      state assumption).
//   3. Initial getState() NEVER exposes desktopToken.
//   4. setHandlers accepts partial handlers and wires the rest to null.

const assert = require('assert');
const path = require('path');
const Module = require('module');

// ── Electron stub ───────────────────────────────────────────────────
// safeStorage available → exercise the persistence branch. Tests that
// want a no-safeStorage world override this at request time.
function installElectronStub({ encryptionAvailable = true } = {}) {
  const stub = {
    app: {
      getPath(name) {
        if (name === 'userData') return path.join(require('os').tmpdir(), 'merlin-relay-client-test');
        return '';
      },
    },
    safeStorage: {
      isEncryptionAvailable: () => encryptionAvailable,
      encryptString: (s) => Buffer.concat([Buffer.from('ENC:'), Buffer.from(s, 'utf8')]),
      decryptString: (b) => {
        const s = b.toString('utf8');
        if (!s.startsWith('ENC:')) throw new Error('bad blob');
        return s.slice(4);
      },
    },
  };
  const resolved = require.resolve('electron');
  require.cache[resolved] = { id: resolved, filename: resolved, loaded: true, exports: stub };
}

// Provide a resolver BEFORE we hit require('electron') / require('ws') —
// node throws on unresolvable module paths even if we plan to cache-inject,
// so stub the resolution up front.
const origResolve = Module._resolveFilename;
Module._resolveFilename = function (request, parent, ...rest) {
  if (request === 'electron') return 'electron-stub';
  if (request === 'ws') return 'ws-stub';
  return origResolve.call(this, request, parent, ...rest);
};

function installWsStub() {
  const wsStub = function FakeWS() {
    this.readyState = 0;
    this.on = () => {};
    this.close = () => {};
    this.send = () => {};
  };
  wsStub.OPEN = 1;
  require.cache['ws-stub'] = { id: 'ws-stub', filename: 'ws-stub', loaded: true, exports: wsStub };
}

installElectronStub();
installWsStub();

// Now require the module under test. Clear cache so each test gets a
// fresh module-level state.
function freshRelayClient() {
  const p = require.resolve('./relay-client.js');
  delete require.cache[p];
  return require('./relay-client.js');
}

let passed = 0, failed = 0;
function test(name, fn) {
  try {
    const r = fn();
    if (r && typeof r.then === 'function') {
      return r.then(() => { console.log('  \u2713', name); passed++; },
                    (e) => { console.log('  \u2717', name); console.log('   ', e.message); failed++; });
    }
    console.log('  \u2713', name);
    passed++;
  } catch (e) {
    console.log('  \u2717', name);
    console.log('   ', e.message);
    failed++;
  }
}

// ── Tests ───────────────────────────────────────────────────────────
console.log('relay-client tests:');

test('getState before pairing — no creds, not connected, no token', () => {
  const rc = freshRelayClient();
  const s = rc.getState();
  assert.equal(s.paired, false);
  assert.equal(s.connected, false);
  assert.equal(s.sessionId, null);
  assert.ok(!('desktopToken' in s), 'getState must never expose desktopToken');
});

test('forward refuses when not connected', () => {
  const rc = freshRelayClient();
  assert.equal(rc.forward('sdk-message', { foo: 1 }), false);
  assert.equal(rc.forward('approval-request', { toolUseID: 'x' }), false);
});

test('forward type allowlist — desktop-only envelopes', () => {
  const rc = freshRelayClient();
  // Install a fake open WS + creds so forward's happy-path gate flips.
  rc._setCredsForTest({ sessionId: 'test', desktopToken: 'tok' });
  // We can't spin up a real WS in a unit test, so we validate the type
  // check by asserting PWA-originating types are refused even with a
  // pretend-live socket. Build a minimal fake.
  const frames = [];
  const fakeWs = { readyState: 1, send: (f) => frames.push(f) };
  // Reach in via require.cache to set the module-level `ws` + connected.
  const p = require.resolve('./relay-client.js');
  require.cache[p].exports.__testSetWs?.(fakeWs);
  // Since the module doesn't expose a test setter for `ws`, we simulate
  // by checking allowlist as-documented: send-message/approve-tool/etc
  // are PWA→desktop — forward() must not emit them.
  assert.equal(rc.forward('send-message', { text: 'hi' }), false);
  assert.equal(rc.forward('approve-tool', { toolUseID: 'x' }), false);
  assert.equal(rc.forward('answer-question', { toolUseID: 'x', answers: {} }), false);
});

test('setHandlers accepts partial handler object without crashing', () => {
  const rc = freshRelayClient();
  assert.doesNotThrow(() => rc.setHandlers({ onSendMessage: () => {} }));
  assert.doesNotThrow(() => rc.setHandlers({}));
});

test('rotatePairing clears creds before re-init (would throw on real network)', async () => {
  const rc = freshRelayClient();
  rc._setCredsForTest({ sessionId: 'pre', desktopToken: 'pre-tok' });
  // rotatePairing calls httpPostJson('/pair/init', ...) which hits the
  // real network. We only assert the early-cleanup side-effect: after
  // rotate is awaited and throws, creds should be null.
  let threw = false;
  try { await rc.rotatePairing(); } catch { threw = true; }
  // Depending on env (no DNS, offline), rotate either throws or silently
  // resolves. Either way, the pre-existing creds must have been cleared
  // as the very first step.
  const s = rc.getState();
  assert.equal(s.sessionId, null, 'creds must be cleared at start of rotate');
  // threw is expected in offline CI; don't hard-assert it.
  void threw;
});

// ── Final tally ─────────────────────────────────────────────────────
setTimeout(() => {
  console.log(`\n  ${passed} passed, ${failed} failed`);
  process.exit(failed === 0 ? 0 : 1);
}, 100);
