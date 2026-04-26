'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');

const { createSelection, keysInsideRect, __rectsIntersect } = require('./archive-multiselect');

function build(keys) {
  const sel = createSelection();
  sel.syncOrder(keys);
  return sel;
}

test('plain click replaces selection with the clicked item', () => {
  const sel = build(['a', 'b', 'c']);
  sel.click('a', {});
  assert.deepEqual(sel.toArray(), ['a']);
  sel.click('b', {});
  assert.deepEqual(sel.toArray(), ['b']);
  assert.equal(sel.anchor(), 'b');
});

test('ctrl-click toggles individual items', () => {
  const sel = build(['a', 'b', 'c']);
  sel.click('a', { ctrlKey: true });
  sel.click('c', { ctrlKey: true });
  assert.deepEqual(sel.toArray(), ['a', 'c']);
  sel.click('a', { ctrlKey: true });
  assert.deepEqual(sel.toArray(), ['c']);
});

test('meta-click is treated identically to ctrl-click (Mac)', () => {
  const sel = build(['a', 'b', 'c']);
  sel.click('a', { metaKey: true });
  sel.click('c', { metaKey: true });
  assert.deepEqual(sel.toArray(), ['a', 'c']);
});

test('shift-click selects an inclusive range from anchor', () => {
  const sel = build(['a', 'b', 'c', 'd', 'e']);
  sel.click('b', {});
  sel.click('d', { shiftKey: true });
  assert.deepEqual(sel.toDocumentOrder(), ['b', 'c', 'd']);
});

test('shift-click works in reverse order (anchor below clicked)', () => {
  const sel = build(['a', 'b', 'c', 'd', 'e']);
  sel.click('d', {});
  sel.click('a', { shiftKey: true });
  assert.deepEqual(sel.toDocumentOrder(), ['a', 'b', 'c', 'd']);
});

test('shift-click with no anchor falls back to plain click', () => {
  const sel = build(['a', 'b', 'c']);
  sel.click('b', { shiftKey: true });
  assert.deepEqual(sel.toArray(), ['b']);
  assert.equal(sel.anchor(), 'b');
});

test('shift-click replaces selection (no ctrl)', () => {
  const sel = build(['a', 'b', 'c', 'd']);
  sel.click('a', { ctrlKey: true });
  sel.click('c', { ctrlKey: true });
  // selection: {a, c}, anchor: c
  sel.click('d', { shiftKey: true });
  // Shift without ctrl REPLACES with [c..d]
  assert.deepEqual(sel.toDocumentOrder(), ['c', 'd']);
});

test('ctrl + shift-click ADDS the range without clearing', () => {
  const sel = build(['a', 'b', 'c', 'd']);
  sel.click('a', { ctrlKey: true });
  // selection: {a}, anchor: a
  sel.click('c', { ctrlKey: true });
  // selection: {a, c}, anchor: c
  sel.click('d', { shiftKey: true, ctrlKey: true });
  // Ctrl+Shift adds [c..d] to existing selection, preserving 'a'.
  assert.deepEqual(sel.toDocumentOrder(), ['a', 'c', 'd']);
});

test('shift-click pivots around the same anchor on repeat', () => {
  const sel = build(['a', 'b', 'c', 'd', 'e']);
  sel.click('c', {});
  sel.click('e', { shiftKey: true });
  assert.deepEqual(sel.toDocumentOrder(), ['c', 'd', 'e']);
  sel.click('a', { shiftKey: true });
  // Anchor stayed at 'c' — second shift-click goes from 'c' to 'a'.
  assert.deepEqual(sel.toDocumentOrder(), ['a', 'b', 'c']);
});

test('selectAll selects every visible key', () => {
  const sel = build(['a', 'b', 'c']);
  sel.selectAll();
  assert.deepEqual(sel.toDocumentOrder(), ['a', 'b', 'c']);
});

test('clear empties selection and anchor', () => {
  const sel = build(['a', 'b', 'c']);
  sel.click('a', {});
  sel.click('b', { ctrlKey: true });
  sel.clear();
  assert.equal(sel.size(), 0);
  assert.equal(sel.anchor(), null);
});

test('syncOrder drops selections for vanished items', () => {
  const sel = build(['a', 'b', 'c']);
  sel.click('a', {});
  sel.click('b', { ctrlKey: true });
  sel.syncOrder(['a', 'c']); // 'b' was deleted from disk
  assert.deepEqual(sel.toArray(), ['a']);
});

test('syncOrder clears anchor if the anchored item vanished', () => {
  const sel = build(['a', 'b', 'c']);
  sel.click('b', {});
  sel.syncOrder(['a', 'c']);
  assert.equal(sel.anchor(), null);
});

test('click on unknown key is a no-op', () => {
  const sel = build(['a', 'b']);
  sel.click('a', {});
  sel.click('zzz', { ctrlKey: true });
  assert.deepEqual(sel.toArray(), ['a']);
});

test('onChange listener fires after each mutation', () => {
  const sel = build(['a', 'b']);
  let count = 0;
  const unsub = sel.onChange(() => { count++; });
  sel.click('a', {});
  sel.click('b', { ctrlKey: true });
  sel.clear();
  assert.equal(count, 3);
  unsub();
  sel.click('a', {});
  assert.equal(count, 3);
});

test('onChange listener throws do not break others', () => {
  const sel = build(['a']);
  let saw = 0;
  sel.onChange(() => { throw new Error('boom'); });
  sel.onChange(() => { saw++; });
  // console.warn from the catch block — silence for test output cleanliness.
  const origWarn = console.warn;
  console.warn = () => {};
  try {
    sel.click('a', {});
  } finally {
    console.warn = origWarn;
  }
  assert.equal(saw, 1);
});

test('toDocumentOrder returns items in grid order, not click order', () => {
  const sel = build(['a', 'b', 'c', 'd']);
  sel.click('d', {});
  sel.click('a', { ctrlKey: true });
  sel.click('c', { ctrlKey: true });
  assert.deepEqual(sel.toDocumentOrder(), ['a', 'c', 'd']);
});

test('setRange additive=false replaces, additive=true adds', () => {
  const sel = build(['a', 'b', 'c', 'd']);
  sel.click('a', {});
  sel.setRange('c', 'd', false);
  assert.deepEqual(sel.toDocumentOrder(), ['c', 'd']);
  sel.setRange('a', 'b', true);
  assert.deepEqual(sel.toDocumentOrder(), ['a', 'b', 'c', 'd']);
});

test('rectsIntersect handles edge cases (touching = no intersect)', () => {
  // Two rects sharing only an edge are NOT considered intersecting.
  // Drag-rect convention — items must overlap, not just touch.
  assert.equal(__rectsIntersect(
    { left: 0, top: 0, right: 10, bottom: 10 },
    { left: 10, top: 0, right: 20, bottom: 10 }
  ), false);
  assert.equal(__rectsIntersect(
    { left: 0, top: 0, right: 10, bottom: 10 },
    { left: 5, top: 5, right: 15, bottom: 15 }
  ), true);
  assert.equal(__rectsIntersect(
    { left: 0, top: 0, right: 10, bottom: 10 },
    { left: 100, top: 100, right: 200, bottom: 200 }
  ), false);
});

test('keysInsideRect filters keys whose box overlaps the drag rect', () => {
  const boxes = {
    a: { left: 0, top: 0, right: 50, bottom: 50 },     // overlaps drag
    b: { left: 200, top: 0, right: 250, bottom: 50 },  // far right — no
    c: { left: 0, top: 200, right: 50, bottom: 250 },  // far below — no
    d: { left: 30, top: 30, right: 80, bottom: 80 },   // overlaps drag
  };
  const dragRect = { left: 25, top: 25, right: 75, bottom: 75 };
  const inside = keysInsideRect(dragRect, (k) => boxes[k], ['a', 'b', 'c', 'd']);
  assert.deepEqual(inside.sort(), ['a', 'd']);
});

test('keysInsideRect skips keys whose getter throws', () => {
  const boxes = {
    a: { left: 0, top: 0, right: 50, bottom: 50 },
  };
  const inside = keysInsideRect(
    { left: 0, top: 0, right: 100, bottom: 100 },
    (k) => { if (k === 'b') throw new Error('boom'); return boxes[k]; },
    ['a', 'b']
  );
  assert.deepEqual(inside, ['a']);
});
