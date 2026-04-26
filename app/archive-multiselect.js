// Merlin archive multi-select state machine.
//
// The Archive panel needs Lightroom-grade culling: scroll a thousand
// generated creatives, multi-select the rejects, trash them, keep moving.
// Single-cap-at-2 selection (the legacy behavior, preserved for the "merge
// two creatives" feature) does not survive that workflow.
//
// This module owns the pure state — no DOM, no IPC, no listeners. It
// returns a Selection instance whose API is what the renderer wires to mouse
// + keyboard events. Tests can drive every interaction without a browser.
//
// Anchor pattern:
//   - Plain click on item N: clear all → select N → anchor = N.
//   - Ctrl/Meta-click on N:  toggle N's selection → anchor = N if added,
//                            anchor = previous anchor if removed.
//   - Shift-click on N:      replace selection with the range [anchor, N]
//                            (inclusive). If no anchor, treat as plain click.
//   - Ctrl/Meta + Shift-click on N: ADD the range [anchor, N] to the
//                            existing selection (additive range).
//   - selectAll():           selects every key the renderer hands in (the
//                            "visible filtered set"). Anchor unchanged.
//   - clear():               empty selection, anchor null.
//
// Items are identified by an opaque string key (the renderer passes the
// archive item's `folder` or composite identifier). The state machine never
// peeks inside the key — it only knows the ordered list it was given.
//
// Listeners: callers register `onChange(selection => …)`. Fired after every
// mutation. Sync, single-shot per mutation; if a listener throws, others
// still fire and the error is logged via console.warn (renderer continues).

'use strict';

// Dual-module shim: window.MerlinArchiveSelection in the renderer, CommonJS
// in the node-based test harness. Mirrors archive-campaign-group.js.
(function (root, factory) {
  const api = factory();
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  } else {
    root.MerlinArchiveSelection = api;
  }
}(typeof self !== 'undefined' ? self : this, function () {

class ArchiveSelection {
  constructor() {
    // Insertion-ordered set of selected keys.
    this._selected = new Set();
    // Caller's most recent flat list of keys (defines the order shift-range
    // walks through). Updated whenever the renderer rebuilds the grid.
    this._orderedKeys = [];
    this._keyIndex = new Map();
    this._anchor = null;
    this._listeners = new Set();
  }

  // The renderer calls this every time it (re)builds the archive grid so the
  // state machine knows the canonical order for shift-range operations and
  // can drop selections for items that no longer exist (e.g. after a delete
  // or a filter change).
  syncOrder(keys) {
    if (!Array.isArray(keys)) keys = [];
    this._orderedKeys = keys.slice();
    this._keyIndex = new Map();
    for (let i = 0; i < keys.length; i++) {
      this._keyIndex.set(keys[i], i);
    }
    let mutated = false;
    for (const k of Array.from(this._selected)) {
      if (!this._keyIndex.has(k)) {
        this._selected.delete(k);
        mutated = true;
      }
    }
    if (this._anchor != null && !this._keyIndex.has(this._anchor)) {
      this._anchor = null;
      mutated = true;
    }
    if (mutated) this._emit();
  }

  // The single dispatcher. Pass the click target's key plus the modifier
  // flags from the MouseEvent. Returns the new selection size.
  click(key, mods) {
    if (!this._keyIndex.has(key)) return this.size();
    const ctrl = !!(mods && (mods.ctrlKey || mods.metaKey));
    const shift = !!(mods && mods.shiftKey);

    if (shift && this._anchor != null && this._keyIndex.has(this._anchor)) {
      const range = this._range(this._anchor, key);
      if (!ctrl) this._selected.clear();
      for (const k of range) this._selected.add(k);
      // Anchor stays put on shift-click — repeated shift-clicks pivot
      // around the same anchor, like every file manager + photo app.
    } else if (ctrl) {
      if (this._selected.has(key)) {
        this._selected.delete(key);
        // Anchor moves to the most recently added item if the removed one
        // was the anchor; otherwise stays.
        if (this._anchor === key) {
          this._anchor = this._lastSelected();
        }
      } else {
        this._selected.add(key);
        this._anchor = key;
      }
    } else {
      // Plain click — replace selection with just this item.
      this._selected.clear();
      this._selected.add(key);
      this._anchor = key;
    }
    this._emit();
    return this.size();
  }

  // Programmatic select-without-event (e.g. drag-rect, keyboard arrow nav
  // with shift held). `additive` controls whether the existing selection
  // is preserved (true = add, false = replace).
  setRange(fromKey, toKey, additive) {
    if (!this._keyIndex.has(fromKey) || !this._keyIndex.has(toKey)) return this.size();
    const range = this._range(fromKey, toKey);
    if (!additive) this._selected.clear();
    for (const k of range) this._selected.add(k);
    this._anchor = fromKey;
    this._emit();
    return this.size();
  }

  selectAll() {
    let mutated = false;
    for (const k of this._orderedKeys) {
      if (!this._selected.has(k)) {
        this._selected.add(k);
        mutated = true;
      }
    }
    if (mutated) this._emit();
    return this.size();
  }

  clear() {
    if (this._selected.size === 0 && this._anchor == null) return 0;
    this._selected.clear();
    this._anchor = null;
    this._emit();
    return 0;
  }

  has(key) {
    return this._selected.has(key);
  }

  size() {
    return this._selected.size;
  }

  // Stable insertion-ordered array. Useful when a caller wants the order
  // the user actually selected things in (for "first selected" semantics).
  toArray() {
    return Array.from(this._selected);
  }

  // Document-order array — the order items currently appear in the grid.
  // Useful for bulk operations that should run top-to-bottom (e.g. trash).
  toDocumentOrder() {
    const arr = [];
    for (const k of this._orderedKeys) {
      if (this._selected.has(k)) arr.push(k);
    }
    return arr;
  }

  anchor() {
    return this._anchor;
  }

  onChange(fn) {
    if (typeof fn !== 'function') return () => {};
    this._listeners.add(fn);
    return () => this._listeners.delete(fn);
  }

  // ── Internals ────────────────────────────────────────────────

  _range(fromKey, toKey) {
    const a = this._keyIndex.get(fromKey);
    const b = this._keyIndex.get(toKey);
    const lo = Math.min(a, b);
    const hi = Math.max(a, b);
    const out = [];
    for (let i = lo; i <= hi; i++) out.push(this._orderedKeys[i]);
    return out;
  }

  _lastSelected() {
    let last = null;
    for (const k of this._selected) last = k;
    return last;
  }

  _emit() {
    for (const fn of Array.from(this._listeners)) {
      try { fn(this); }
      catch (err) {
        try { console.warn('[archive-multiselect] listener threw', err); } catch {}
      }
    }
  }
}

function createSelection() {
  return new ArchiveSelection();
}

// Drag-rectangle helpers. The renderer hands us the live drag rect (in
// document coordinates) and a function that maps a key to its bounding box
// so we can compute the intersection without coupling to the DOM here.
function keysInsideRect(rect, getRectForKey, keys) {
  const out = [];
  if (!rect || !Array.isArray(keys)) return out;
  for (const key of keys) {
    let box;
    try { box = getRectForKey(key); } catch { box = null; }
    if (!box) continue;
    if (rectsIntersect(rect, box)) out.push(key);
  }
  return out;
}

function rectsIntersect(a, b) {
  return !(b.left >= a.right || b.right <= a.left || b.top >= a.bottom || b.bottom <= a.top);
}

return {
  ArchiveSelection,
  createSelection,
  keysInsideRect,
  // Exposed for tests only.
  __rectsIntersect: rectsIntersect,
};

}));
