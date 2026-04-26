// Merlin results/ watcher.
//
// The Archive needs live invalidation: when the Go binary writes a fresh
// run folder, when the user trashes a card, or when an external tool drops
// a file into results/, the renderer should know within ~1s so the grid
// reflects truth without a manual refresh.
//
// We don't add a chokidar dependency — Node's built-in `fs.watch` with
// `recursive: true` covers the two production targets (Windows / macOS)
// natively. On Linux fs.watch lacks recursive support; we degrade to a
// 5s polling loop that diffs the top-level folder listing. Linux is not
// a shipping target today (no NSIS/DMG equivalent), but the fallback
// keeps `node --test` runnable on any CI host.
//
// API:
//   const w = createResultsWatcher(resultsDir, { onChange: (events) => {} });
//   w.start();
//   w.stop();
//
// Events are debounced — every burst of fs notifications within
// `debounceMs` (default 400ms) collapses to ONE callback carrying the unique
// set of paths that changed. The renderer uses this to throw out the cached
// archive index and reload, so back-to-back writes don't trigger five
// redundant rebuilds.
//
// Self-write filtering: paths matching `.flags.json` (and its .tmp.* atomic
// rename siblings) are dropped. The flag-toggle UX writes the sidecar from
// the main process; the watcher would otherwise trigger an Archive reload
// on every keystroke flag change, defeating the point of the optimistic
// in-renderer flag state.

'use strict';

const fs = require('fs');
const path = require('path');

const DEFAULT_DEBOUNCE_MS = 400;
const POLL_INTERVAL_MS = 5000;
const SELF_WRITE_PATTERNS = [
  /(?:^|[\\/])\.flags\.json(?:\.tmp\.[^\\/]+)?$/,
  /(?:^|[\\/])archive-index\.json(?:\.tmp\.[^\\/]+)?$/,
];

class ResultsWatcher {
  constructor(resultsDir, opts) {
    if (!resultsDir || typeof resultsDir !== 'string') {
      throw new Error('results-watcher: resultsDir required');
    }
    this._dir = resultsDir;
    this._onChange = (opts && typeof opts.onChange === 'function') ? opts.onChange : () => {};
    this._debounceMs = (opts && Number.isFinite(opts.debounceMs)) ? opts.debounceMs : DEFAULT_DEBOUNCE_MS;
    this._watcher = null;
    this._pollTimer = null;
    this._pollSnapshot = null;
    this._pendingPaths = new Set();
    this._flushTimer = null;
    this._started = false;
  }

  start() {
    if (this._started) return;
    this._started = true;

    // results/ may not exist yet on first launch — create it so the watcher
    // can attach. Idempotent.
    try {
      fs.mkdirSync(this._dir, { recursive: true });
    } catch (err) {
      if (err && err.code !== 'EEXIST') {
        try { console.warn('[results-watcher] mkdir failed', err.message); } catch {}
        return;
      }
    }

    // Try the recursive native watcher first (Win/Mac).
    try {
      this._watcher = fs.watch(this._dir, { recursive: true, persistent: false }, (_eventType, filename) => {
        if (!filename) return;
        const full = path.join(this._dir, filename);
        if (this._isSelfWrite(full)) return;
        this._pendingPaths.add(full);
        this._scheduleFlush();
      });
      this._watcher.on('error', (err) => {
        try { console.warn('[results-watcher] watch error', err.message); } catch {}
      });
      return;
    } catch (err) {
      // Linux + no recursive support — fall back to polling.
      this._watcher = null;
    }

    this._startPolling();
  }

  stop() {
    if (!this._started) return;
    this._started = false;
    if (this._watcher) {
      try { this._watcher.close(); } catch {}
      this._watcher = null;
    }
    if (this._pollTimer) {
      clearInterval(this._pollTimer);
      this._pollTimer = null;
    }
    if (this._flushTimer) {
      clearTimeout(this._flushTimer);
      this._flushTimer = null;
    }
    this._pendingPaths.clear();
    this._pollSnapshot = null;
  }

  // ── Internals ────────────────────────────────────────────────

  _isSelfWrite(fullPath) {
    return SELF_WRITE_PATTERNS.some(re => re.test(fullPath));
  }

  _scheduleFlush() {
    if (this._flushTimer) return;
    this._flushTimer = setTimeout(() => {
      this._flushTimer = null;
      const paths = Array.from(this._pendingPaths);
      this._pendingPaths.clear();
      if (paths.length === 0) return;
      try { this._onChange(paths); }
      catch (err) {
        try { console.warn('[results-watcher] onChange threw', err); } catch {}
      }
    }, this._debounceMs);
  }

  _startPolling() {
    this._pollSnapshot = this._snapshotDir();
    this._pollTimer = setInterval(() => {
      const next = this._snapshotDir();
      if (!shallowEqual(this._pollSnapshot, next)) {
        this._pollSnapshot = next;
        this._pendingPaths.add(this._dir);
        this._scheduleFlush();
      }
    }, POLL_INTERVAL_MS);
  }

  _snapshotDir() {
    const out = new Map();
    let entries;
    try {
      entries = fs.readdirSync(this._dir, { withFileTypes: true });
    } catch {
      return out;
    }
    for (const ent of entries) {
      if (this._isSelfWrite(ent.name)) continue;
      try {
        const stat = fs.statSync(path.join(this._dir, ent.name));
        out.set(ent.name, stat.mtimeMs + ':' + stat.size);
      } catch {}
    }
    return out;
  }
}

function shallowEqual(a, b) {
  if (!a || !b) return false;
  if (a.size !== b.size) return false;
  for (const [k, v] of a) {
    if (b.get(k) !== v) return false;
  }
  return true;
}

function createResultsWatcher(resultsDir, opts) {
  return new ResultsWatcher(resultsDir, opts);
}

module.exports = {
  ResultsWatcher,
  createResultsWatcher,
  DEFAULT_DEBOUNCE_MS,
  // Exposed for tests only.
  __SELF_WRITE_PATTERNS: SELF_WRITE_PATTERNS,
};
