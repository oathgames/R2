// Tests for app/captions.js — pure functions only.
//
// We intentionally do NOT spawn ffmpeg/whisper-cli in tests:
//   - The bundled toolchain isn't present in CI nor on every contributor
//     machine; a passing CI run shouldn't depend on 200MB of voice tools.
//   - Subprocess invocation is the most network/IO-heavy part of the flow
//     and would balloon test time from milliseconds to seconds-per-test.
//   - The most fragile parts of the implementation (centisecond rounding,
//     ASS escaping, word-window edge cases, time monotonicity) are 100%
//     pure-function logic and fully testable in isolation.
//
// Run: `node app/captions.test.js`. Uses Node's built-in test runner,
// matching the convention of every other *.test.js file in app/.

'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const fs = require('node:fs');
const os = require('node:os');

const captions = require('./captions');
const {
  parseWhisperWordsJson,
  buildAssSubtitles,
  buildContextBubble,
  validateInput,
  findVoiceTools,
  formatAssTime,
  escapeAssText,
  COLOR_YELLOW,
  COLOR_WHITE,
  COLOR_BLACK,
} = captions;

// ── formatAssTime ────────────────────────────────────────────

test('formatAssTime emits H:MM:SS.cc with centiseconds (NOT milliseconds)', () => {
  // Common bug: shipping milliseconds into the .cc field. libass parses
  // only the first two digits and silently truncates, which is the
  // exact failure mode this test exists to catch.
  assert.equal(formatAssTime(0), '0:00:00.00');
  assert.equal(formatAssTime(1500), '0:00:01.50');
  assert.equal(formatAssTime(60000), '0:01:00.00');
  assert.equal(formatAssTime(3661230), '1:01:01.23');
});

test('formatAssTime rounds milliseconds to centiseconds', () => {
  assert.equal(formatAssTime(1234), '0:00:01.23');
  assert.equal(formatAssTime(1235), '0:00:01.24');   // round-half-to-even or up; assert one
  assert.equal(formatAssTime(999), '0:00:01.00');    // 99.9cs rounds to 100cs → carries to seconds
});

test('formatAssTime clamps negative / NaN to zero', () => {
  assert.equal(formatAssTime(-100), '0:00:00.00');
  assert.equal(formatAssTime(NaN), '0:00:00.00');
  assert.equal(formatAssTime(undefined), '0:00:00.00');
});

// ── escapeAssText ────────────────────────────────────────────

test('escapeAssText escapes braces and backslashes', () => {
  assert.equal(escapeAssText('hello'), 'hello');
  assert.equal(escapeAssText('a{b}c'), 'a\\{b\\}c');
  assert.equal(escapeAssText('a\\b'), 'a\\\\b');
  assert.equal(escapeAssText('a\nb'), 'a b');
});

// ── parseWhisperWordsJson ────────────────────────────────────

test('parseWhisperWordsJson extracts word list with ms offsets', () => {
  const json = {
    transcription: [
      { text: ' Hello', offsets: { from: 0, to: 320 } },
      { text: ' world', offsets: { from: 320, to: 720 } },
    ],
  };
  const words = parseWhisperWordsJson(json);
  assert.equal(words.length, 2);
  assert.deepEqual(words[0], { text: 'Hello', startMs: 0, endMs: 320 });
  assert.deepEqual(words[1], { text: 'world', startMs: 320, endMs: 720 });
});

test('parseWhisperWordsJson skips whisper diagnostic markers', () => {
  const json = {
    transcription: [
      { text: '[BLANK_AUDIO]', offsets: { from: 0, to: 200 } },
      { text: ' Hello', offsets: { from: 200, to: 500 } },
      { text: '[MUSIC]', offsets: { from: 500, to: 700 } },
    ],
  };
  const words = parseWhisperWordsJson(json);
  assert.equal(words.length, 1);
  assert.equal(words[0].text, 'Hello');
});

test('parseWhisperWordsJson skips pure-punctuation segments', () => {
  const json = {
    transcription: [
      { text: ' Hi', offsets: { from: 0, to: 200 } },
      { text: '.',  offsets: { from: 200, to: 250 } },
      { text: ' there', offsets: { from: 250, to: 600 } },
    ],
  };
  const words = parseWhisperWordsJson(json);
  assert.equal(words.length, 2);
  assert.deepEqual(words.map(w => w.text), ['Hi', 'there']);
});

test('parseWhisperWordsJson clamps negative startMs and floors zero-length to 200ms', () => {
  const json = {
    transcription: [
      { text: 'Hi', offsets: { from: -50, to: -10 } },
      { text: 'there', offsets: { from: 200, to: 200 } },
    ],
  };
  const words = parseWhisperWordsJson(json);
  assert.equal(words.length, 2);
  assert.equal(words[0].startMs, 0);
  assert.ok(words[0].endMs > words[0].startMs, 'endMs must be > startMs');
  assert.ok(words[1].endMs > words[1].startMs, 'endMs must be > startMs');
});

test('parseWhisperWordsJson keeps timestamps monotonic across words', () => {
  // If whisper emits an out-of-order segment (broken model output), the
  // ASS file would otherwise have overlapping/out-of-order Dialogue
  // events, which libass renders weirdly. We clamp startMs to >= prevEnd.
  const json = {
    transcription: [
      { text: 'A', offsets: { from: 100, to: 500 } },
      { text: 'B', offsets: { from: 200, to: 300 } },   // overlaps prev
    ],
  };
  const words = parseWhisperWordsJson(json);
  assert.equal(words.length, 2);
  assert.ok(words[1].startMs >= words[0].endMs, 'second word must start at or after first ends');
});

test('parseWhisperWordsJson returns [] on malformed input', () => {
  assert.deepEqual(parseWhisperWordsJson(null), []);
  assert.deepEqual(parseWhisperWordsJson(undefined), []);
  assert.deepEqual(parseWhisperWordsJson({}), []);
  assert.deepEqual(parseWhisperWordsJson({ transcription: 'not-an-array' }), []);
});

// ── buildContextBubble ───────────────────────────────────────
//
// Window rule: `<prev>  <ACTIVE yellow>  <next>`. The first word has
// no prev (returns ''); the last word has no next (returns ''). This
// keeps a 3-word context bubble for the bulk of the video while
// degrading cleanly at the edges.

test('buildContextBubble returns prev/active/next for middle word', () => {
  const words = [
    { text: 'a', startMs: 0, endMs: 100 },
    { text: 'b', startMs: 100, endMs: 200 },
    { text: 'c', startMs: 200, endMs: 300 },
    { text: 'd', startMs: 300, endMs: 400 },
    { text: 'e', startMs: 400, endMs: 500 },
  ];
  // Active = c (index 2): bubble = b, c-yellow, d.
  const bubble = buildContextBubble(words, 2);
  assert.deepEqual(bubble, { prev: 'b', active: 'c', next: 'd' });
});

test('buildContextBubble has empty prev for first word', () => {
  const words = [
    { text: 'first', startMs: 0, endMs: 100 },
    { text: 'second', startMs: 100, endMs: 200 },
  ];
  const bubble = buildContextBubble(words, 0);
  assert.equal(bubble.prev, '');
  assert.equal(bubble.active, 'first');
  assert.equal(bubble.next, 'second');
});

test('buildContextBubble has empty next for last word', () => {
  const words = [
    { text: 'first', startMs: 0, endMs: 100 },
    { text: 'last', startMs: 100, endMs: 200 },
  ];
  const bubble = buildContextBubble(words, 1);
  assert.equal(bubble.prev, 'first');
  assert.equal(bubble.active, 'last');
  assert.equal(bubble.next, '');
});

test('buildContextBubble single word: empty prev and next', () => {
  const words = [{ text: 'solo', startMs: 0, endMs: 500 }];
  const bubble = buildContextBubble(words, 0);
  assert.deepEqual(bubble, { prev: '', active: 'solo', next: '' });
});

// ── buildAssSubtitles ────────────────────────────────────────

test('buildAssSubtitles produces a complete ASS file with header + Dialogue events', () => {
  const words = [
    { text: 'Hello', startMs: 0, endMs: 500 },
    { text: 'world', startMs: 500, endMs: 1000 },
  ];
  const ass = buildAssSubtitles({ words, playResX: 1080, playResY: 1920 });
  assert.match(ass, /\[Script Info\]/);
  assert.match(ass, /\[V4\+ Styles\]/);
  assert.match(ass, /\[Events\]/);
  assert.match(ass, /PlayResX: 1080/);
  assert.match(ass, /PlayResY: 1920/);
  // One Dialogue line per word.
  const dialogueLines = ass.split('\n').filter(l => l.startsWith('Dialogue:'));
  assert.equal(dialogueLines.length, 2);
});

test('buildAssSubtitles encodes the active word in yellow with white-resetting tag', () => {
  const words = [
    { text: 'punch', startMs: 0, endMs: 500 },
    { text: 'word', startMs: 500, endMs: 1000 },
  ];
  const ass = buildAssSubtitles({ words });
  // Expect each Dialogue line to contain {\c&H0000FFFF&}<word>{\r}.
  const yellowOpen = `{\\c${COLOR_YELLOW}}`;
  assert.ok(ass.includes(`${yellowOpen}punch{\\r}`), `expected yellow-wrapped active word, got:\n${ass}`);
  assert.ok(ass.includes(`${yellowOpen}word{\\r}`), `expected yellow-wrapped active word, got:\n${ass}`);
});

test('buildAssSubtitles default style is bold sans-serif with bottom-center alignment', () => {
  const words = [{ text: 'x', startMs: 0, endMs: 100 }];
  const ass = buildAssSubtitles({ words, playResY: 1920 });
  // Style line: Style: Default,<font>,<size>,...,Bold=-1,...,Alignment=2
  const styleLine = ass.split('\n').find(l => l.startsWith('Style: Default'));
  assert.ok(styleLine, 'style line present');
  // Bold flag = -1 (true) sits in field 8.
  const fields = styleLine.replace(/^Style:\s*/, '').split(',');
  // Format: Name=0, Fontname=1, Fontsize=2, PrimaryColour=3, SecondaryColour=4,
  //         OutlineColour=5, BackColour=6, Bold=7, Italic=8, Underline=9,
  //         StrikeOut=10, ScaleX=11, ScaleY=12, Spacing=13, Angle=14,
  //         BorderStyle=15, Outline=16, Shadow=17, Alignment=18.
  assert.equal(fields[7], '-1', 'Bold must be -1 (true)');
  assert.equal(fields[18], '2', 'Alignment must be 2 (bottom-center)');
  // Font size is ~14% of playResY (≈ 7.2% in our fixed coefficient)
  // — at 1920px this is 138px. Just check it's > 24 (legibility floor).
  assert.ok(parseInt(fields[2], 10) >= 24, `fontSize must be ≥ 24, got ${fields[2]}`);
  // Fontname includes Arial Black (the default Hormozi pick).
  assert.match(fields[1], /Arial Black/);
});

test('buildAssSubtitles emits centisecond timestamps, never raw ms', () => {
  const words = [{ text: 'x', startMs: 1234, endMs: 5678 }];
  const ass = buildAssSubtitles({ words });
  const dialogueLine = ass.split('\n').find(l => l.startsWith('Dialogue:'));
  assert.ok(dialogueLine);
  // Format: Dialogue: 0,<start>,<end>,Default,...
  const parts = dialogueLine.split(',');
  assert.equal(parts[1], '0:00:01.23', 'start time must be H:MM:SS.cc');
  assert.equal(parts[2], '0:00:05.68', 'end time must be H:MM:SS.cc');
});

test('buildAssSubtitles renders 3-word context bubble for middle words', () => {
  const words = ['a', 'b', 'c', 'd', 'e'].map((t, i) => ({
    text: t, startMs: i * 100, endMs: (i + 1) * 100,
  }));
  const ass = buildAssSubtitles({ words });
  const dialogueLines = ass.split('\n').filter(l => l.startsWith('Dialogue:'));
  // For active=c (line index 2), the rendered text should be:
  //     b {\c<yellow>}c{\r} d
  const cLine = dialogueLines[2];
  assert.ok(cLine, 'c line present');
  // Strip everything before the text field (Dialogue: 0,<start>,<end>,Style,Name,L,R,V,Effect,<text...>)
  // ASS Dialogue has 9 leading comma fields then the text — the text may
  // itself contain commas (style overrides), but our active-word
  // injection has no commas inside the override.
  const textField = cLine.split(',').slice(9).join(',');
  // The bubble should mention b, c (yellow), and d.
  assert.match(textField, /^b\s+\{\\c&H0000FFFF&\}c\{\\r\}\s+d$/);
});

test('buildAssSubtitles handles empty word list (header-only)', () => {
  const ass = buildAssSubtitles({ words: [] });
  assert.match(ass, /\[Events\]/);
  // No Dialogue lines.
  const dialogueLines = ass.split('\n').filter(l => l.startsWith('Dialogue:'));
  assert.equal(dialogueLines.length, 0);
});

test('buildAssSubtitles escapes braces in transcript text', () => {
  const words = [{ text: 'a{b}c', startMs: 0, endMs: 100 }];
  const ass = buildAssSubtitles({ words });
  // Expect a\{b\}c inside the dialogue text — NOT the raw `a{b}c` which
  // would be interpreted as a libass override block.
  assert.ok(ass.includes('a\\{b\\}c'), `expected escaped braces, got:\n${ass}`);
});

// ── validateInput ────────────────────────────────────────────

test('validateInput rejects missing / non-absolute / wrong-extension paths', () => {
  let r;
  r = validateInput({ videoPath: '' });
  assert.equal(r.ok, false);
  assert.equal(r.error, 'captions:invalid-input');

  r = validateInput({ videoPath: 'relative/path.mp4' });
  assert.equal(r.ok, false);
  assert.equal(r.error, 'captions:invalid-input');

  // Use a platform-correct absolute path that ends in a bad extension.
  const bad = path.resolve(os.tmpdir(), 'foo.txt');
  r = validateInput({ videoPath: bad });
  assert.equal(r.ok, false);
  assert.equal(r.error, 'captions:invalid-input');
});

test('validateInput rejects non-existent file with not-found code', () => {
  const ghost = path.resolve(os.tmpdir(), `merlin-captions-test-${Date.now()}-${Math.random().toString(36).slice(2)}.mp4`);
  const r = validateInput({ videoPath: ghost });
  assert.equal(r.ok, false);
  assert.equal(r.error, 'captions:not-found');
});

test('validateInput accepts a real .mp4 file in tmpdir', () => {
  const real = path.resolve(os.tmpdir(), `merlin-captions-test-${Date.now()}.mp4`);
  // Write a 4KB placeholder — > 1KB minimum, well under 500MB cap.
  fs.writeFileSync(real, Buffer.alloc(4096));
  try {
    const r = validateInput({ videoPath: real });
    assert.equal(r.ok, true);
    assert.equal(r.sizeBytes, 4096);
  } finally {
    try { fs.unlinkSync(real); } catch {}
  }
});

test('validateInput rejects oversized file', () => {
  // We can't actually create a 500MB file here without burning real disk.
  // Mock the fs module surface validateInput uses.
  const fakeFs = {
    statSync: () => ({ isFile: () => true, size: 600 * 1024 * 1024 }),
  };
  const real = path.resolve(os.tmpdir(), 'fake-large.mp4');
  const r = validateInput({ videoPath: real, fs: fakeFs });
  assert.equal(r.ok, false);
  assert.equal(r.error, 'captions:too-large');
  assert.match(r.errorDetail, /500MB/);
});

test('validateInput rejects suspiciously tiny file', () => {
  const fakeFs = {
    statSync: () => ({ isFile: () => true, size: 100 }),  // < 1KB floor
  };
  const real = path.resolve(os.tmpdir(), 'fake-tiny.mp4');
  const r = validateInput({ videoPath: real, fs: fakeFs });
  assert.equal(r.ok, false);
  assert.equal(r.error, 'captions:invalid-input');
  assert.match(r.errorDetail, /too small/);
});

// ── findVoiceTools ───────────────────────────────────────────

test('findVoiceTools resolves install-first, workspace-fallback', () => {
  // Build a synthetic fs that says: install has ffmpeg only; workspace
  // has whisper-cli + model. Verify each tool resolves to the right
  // location.
  const installRoot = '/install';
  const workspaceRoot = '/workspace';
  const present = new Set([
    `${installRoot}/.claude/tools/ffmpeg.exe`,
    `${workspaceRoot}/.claude/tools/whisper-cli.exe`,
    `${workspaceRoot}/.claude/tools/ggml-small.en-q5_1.bin`,
  ]);
  const fakeFs = {
    existsSync: (p) => present.has(p.replace(/\\/g, '/')),
  };
  // Use POSIX path operations explicitly so the test passes on Windows
  // (where path.join would use backslashes that wouldn't match our
  // forward-slash fixture set).
  const fakePath = require('node:path').posix;
  const tools = findVoiceTools({
    appInstall: installRoot,
    appRoot: workspaceRoot,
    fs: fakeFs,
    path: fakePath,
    isWin: true,
  });
  assert.equal(tools.ffmpegPath, `${installRoot}/.claude/tools/ffmpeg.exe`);
  assert.equal(tools.whisperBin, `${workspaceRoot}/.claude/tools/whisper-cli.exe`);
  assert.equal(tools.modelPath, `${workspaceRoot}/.claude/tools/ggml-small.en-q5_1.bin`);
});

test('findVoiceTools returns null when a tool is missing', () => {
  const fakeFs = { existsSync: () => false };
  const tools = findVoiceTools({
    appInstall: '/install',
    appRoot: '/workspace',
    fs: fakeFs,
    path: require('node:path').posix,
    isWin: false,
  });
  assert.equal(tools.ffmpegPath, null);
  assert.equal(tools.whisperBin, null);
  assert.equal(tools.modelPath, null);
});

// ── Color constants ──────────────────────────────────────────
//
// Asserted to catch a future refactor that swaps to RGB byte order
// (libass uses BGR) — the colors would still SHIP and would silently
// mis-render.

test('color constants are libass &HAABBGGRR& format', () => {
  // White: alpha=00, B=FF, G=FF, R=FF → &H00FFFFFF&
  assert.equal(COLOR_WHITE,  '&H00FFFFFF&');
  // Yellow (R=255, G=255, B=0): alpha=00, B=00, G=FF, R=FF → &H0000FFFF&
  assert.equal(COLOR_YELLOW, '&H0000FFFF&');
  // Black: all zero
  assert.equal(COLOR_BLACK,  '&H00000000&');
});
