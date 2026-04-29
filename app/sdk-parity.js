// app/sdk-parity.js — pure helpers powering the v1.19.5 SDK parity work.
//
// Three families of helpers, each addressing one of the dimensions in the
// 2026-04-29 RSI session that closed the perceived-quality gap with
// Claude.ai / Claude Desktop:
//
//   1. queueBadgeReducer — mid-turn message queue. The renderer subscribes
//      to main.js's 'message-queued' (depth grows) and 'message-queue-drained'
//      (depth shrinks) events and feeds them through this reducer. The
//      reducer returns the badge text to render — or null when the badge
//      should hide. Pure function, fully testable without DOM.
//
//   2. formatPreToolStatus — formats the status sentence Merlin emits before
//      any tool call that takes >15s OR spends real money. Concrete fields:
//      action, count, context (brand / product / refs), and ETA/cost. Used
//      both for renderer-side rendering of the bot's status preview and as
//      the canonical reference for what the agent SHOULD emit.
//
//   3. isRawErrorVerbatim — given a tool-output line, classifies whether
//      the model's surface text appears to be the raw tool error or a
//      paraphrased diagnosis. Powers the confabulation-prevention rule in
//      commands/merlin.md by giving us a programmatic check we can use in
//      future telemetry / heuristic flagging.
//
// All exports are pure (no DOM, no fs, no network). Test file at
// app/sdk-parity.test.js covers every branch.
//
// Dual-module shim — loaded as a global (window.MerlinSdkParity) in the
// renderer, and as a CommonJS module in the Node test harness.

(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory();
  } else {
    root.MerlinSdkParity = factory();
  }
}(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  // ── (1) queueBadgeReducer ───────────────────────────────────────
  //
  // Renderer state machine for the chat-status row queue badge.
  //
  //   state := { depth: number, lastChange: 'queued' | 'drained' | null }
  //
  //   reduce(state, evt) → { state, badge }
  //
  //   evt shape:
  //     { type: 'queued',  depth: <int> }   // main.js fired 'message-queued'
  //     { type: 'drained', depth: <int> }   // main.js fired 'message-queue-drained'
  //     { type: 'reset' }                    // session boundary, brand switch
  //
  //   badge: string | null
  //     null  → hide the badge entirely
  //     str   → render this label inside the badge element
  //
  // Why "queued (1)" instead of "queued"? At depth ≥ 2 the user benefits from
  // knowing they piled up multiple — e.g. "queued (3)" tells them the agent
  // has 3 follow-ups to read after the current tool call. At depth 1 we still
  // show the count for consistency (the alternative — "queued" without a
  // number — looks like a different state).

  function queueBadgeReducer(state, evt) {
    const prev = state || { depth: 0, lastChange: null };
    if (!evt || typeof evt !== 'object') {
      return { state: prev, badge: prev.depth > 0 ? `queued (${prev.depth})` : null };
    }
    if (evt.type === 'reset') {
      return { state: { depth: 0, lastChange: null }, badge: null };
    }
    if (evt.type === 'queued' || evt.type === 'drained') {
      // Defensive: depth might be undefined if the IPC event was malformed.
      // Treat as "preserve prior depth" rather than crashing the renderer.
      let next = typeof evt.depth === 'number' && Number.isFinite(evt.depth) && evt.depth >= 0
        ? Math.floor(evt.depth)
        : prev.depth;
      // Cap at the same 50-message ceiling main.js enforces. Anything higher
      // is a bug — clamp visibly so the user sees "queued (50)" not gibberish.
      if (next > 50) next = 50;
      const newState = { depth: next, lastChange: evt.type };
      return {
        state: newState,
        badge: next > 0 ? `queued (${next})` : null,
      };
    }
    // Unknown event type — preserve state, keep current badge.
    return { state: prev, badge: prev.depth > 0 ? `queued (${prev.depth})` : null };
  }

  // ── (2) formatPreToolStatus ─────────────────────────────────────
  //
  // Render the canonical pre-tool-status sentence. Used as both:
  //   - the single source of truth for what the rule in commands/merlin.md
  //     describes (so agents have a concrete pattern to match), and
  //   - a renderer-side helper for any UI that wants to preview / lint
  //     status sentences for telemetry.
  //
  // Required fields: action, count, context. ETA OR cost (one or both
  // allowed). All fields trimmed; missing required → null (rule fires
  // "STOP and ask first" instead of emitting a half-formed sentence).

  function formatPreToolStatus(opts) {
    const o = opts || {};
    const action = typeof o.action === 'string' ? o.action.trim() : '';
    const count = typeof o.count === 'number' && Number.isFinite(o.count) && o.count > 0
      ? Math.floor(o.count)
      : 0;
    const context = typeof o.context === 'string' ? o.context.trim() : '';
    const eta = typeof o.eta === 'string' ? o.eta.trim() : '';
    const cost = typeof o.cost === 'string' ? o.cost.trim() : '';

    if (!action || !count || !context) return null;
    if (!eta && !cost) return null;

    // Singular / plural — most natural English. action="image" + count=12
    // → "Generating 12 images". action="email" + count=1 → "Generating 1 email".
    const noun = count === 1 ? action : pluralize(action);
    const pieces = [];
    pieces.push(`Generating ${count} ${noun}`);
    pieces.push(`using ${context}`);
    const meta = [];
    if (eta) meta.push(`~${eta}`);
    if (cost) meta.push(`~${cost}`);
    return `${pieces.join(' ')} — ${meta.join(', ')}.`;
  }

  function pluralize(noun) {
    // Handles the small set of nouns Merlin actually pluralizes for
    // pre-tool status: image, video, voiceover, ad, blog, post, email,
    // headline. Intentionally NOT a general English pluralizer — that
    // would invite drift in test expectations every time a new noun is
    // added. Add to the table below as needed.
    const irregular = {
      voice: 'voices',
      voiceover: 'voiceovers',
      headline: 'headlines',
    };
    if (irregular[noun]) return irregular[noun];
    // Default: append "s". Covers image/video/ad/blog/post/email.
    if (noun.endsWith('s') || noun.endsWith('x') || noun.endsWith('ch') || noun.endsWith('sh')) {
      return noun + 'es';
    }
    if (noun.endsWith('y') && noun.length > 1 && !'aeiou'.includes(noun[noun.length - 2])) {
      return noun.slice(0, -1) + 'ies';
    }
    return noun + 's';
  }

  // ── (3) isRawErrorVerbatim ──────────────────────────────────────
  //
  // Heuristic classifier for "is this surface text the raw tool error, or
  // a paraphrased diagnosis?". Used in the confabulation-prevention rule
  // in commands/merlin.md (universal: "When a tool returns an error,
  // surface it verbatim. Never paraphrase into a plausible-sounding root
  // cause without supporting evidence.").
  //
  //   raw:    string — the original error returned by the tool
  //   surface: string — what the agent wrote in chat
  //
  // Returns true when surface contains a substring of raw that's
  // long enough to be plausibly the verbatim error (≥ 24 chars, since
  // most real error strings are at least that long once you include the
  // status code + message). False indicates the agent likely paraphrased.
  //
  // Intentionally conservative — the rule is "verbatim or explicit
  // I-don't-know," so this helper biases toward flagging paraphrasing.

  function isRawErrorVerbatim(raw, surface) {
    if (typeof raw !== 'string' || typeof surface !== 'string') return false;
    if (!raw || !surface) return false;
    const r = raw.trim();
    const s = surface.trim();
    if (!r || !s) return false;
    // Easy case — surface is exactly the raw error (or contains it as a substring).
    if (s.includes(r)) return true;
    // Longer-substring fallback: a 24-char window of the raw error appears
    // verbatim in surface. 24 chars is short enough to permit minor surrounding
    // formatting (quotes, "got: ...", etc.) but long enough to disambiguate
    // from generic English coincidences.
    const minWindow = 24;
    if (r.length < minWindow) {
      // For short raw errors (e.g. "404 Not Found"), require exact substring.
      return s.includes(r);
    }
    for (let i = 0; i + minWindow <= r.length; i++) {
      const slice = r.slice(i, i + minWindow);
      if (s.includes(slice)) return true;
    }
    return false;
  }

  // ── (4) renderQueuedBadgeMessage ────────────────────────────────
  //
  // Builds the optional inline hint shown when the user types a follow-up
  // during a hung tool call. Cosmetic helper for the renderer; centralized
  // here so the test suite locks the copy.

  function renderQueuedBadgeMessage(depth) {
    if (typeof depth !== 'number' || !Number.isFinite(depth) || depth <= 0) return null;
    if (depth === 1) {
      return 'Got your message — finishing the current step first.';
    }
    return `Got your ${depth} messages — finishing the current step first, then I'll work through them in order.`;
  }

  return {
    queueBadgeReducer,
    formatPreToolStatus,
    isRawErrorVerbatim,
    renderQueuedBadgeMessage,
    pluralize,
  };
}));
