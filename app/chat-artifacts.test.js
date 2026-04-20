// Unit tests for chat-artifacts.js. Run with:
//   node app/chat-artifacts.test.js
//
// These helpers back the "auto-embed unreferenced image artifacts" feature
// in renderer.js. The core invariant under test: if the model writes a PNG
// during a turn and forgets to embed it in the final bubble, we still
// surface it — without ever trying to load something the merlin:// protocol
// handler would 403.

const assert = require('assert');
const {
  IMAGE_EXT_RE,
  extractImagePathsFromToolInput,
  normalizeImagePathForMerlinUrl,
  uniqueByBasename,
  bubbleAlreadyReferences,
} = require('./chat-artifacts');

let passed = 0;
let failed = 0;

function test(name, fn) {
  try {
    fn();
    console.log('  \u2713', name);
    passed++;
  } catch (err) {
    console.error('  \u2717', name);
    console.error('   ', err && err.message ? err.message : err);
    failed++;
  }
}

// ── IMAGE_EXT_RE ────────────────────────────────────────────────

test('IMAGE_EXT_RE matches common image extensions', () => {
  for (const ext of ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg']) {
    assert.ok(IMAGE_EXT_RE.test(`foo.${ext}`), `expected match for .${ext}`);
    assert.ok(IMAGE_EXT_RE.test(`FOO.${ext.toUpperCase()}`), `expected case-insensitive match for .${ext}`);
  }
});

test('IMAGE_EXT_RE rejects non-image extensions', () => {
  for (const ext of ['txt', 'md', 'json', 'mp4', 'webm', 'pdf']) {
    assert.ok(!IMAGE_EXT_RE.test(`foo.${ext}`), `expected no match for .${ext}`);
  }
});

// ── extractImagePathsFromToolInput: Write ───────────────────────

test('Write tool: extracts file_path ending in image extension', () => {
  const out = extractImagePathsFromToolInput('Write', { file_path: 'results/chart.png', content: '...' });
  assert.deepStrictEqual(out, ['results/chart.png']);
});

test('Write tool: ignores non-image file_path', () => {
  const out = extractImagePathsFromToolInput('Write', { file_path: 'notes.md', content: '...' });
  assert.deepStrictEqual(out, []);
});

test('Write tool: handles missing input gracefully', () => {
  assert.deepStrictEqual(extractImagePathsFromToolInput('Write', null), []);
  assert.deepStrictEqual(extractImagePathsFromToolInput('Write', {}), []);
  assert.deepStrictEqual(extractImagePathsFromToolInput('Write', { file_path: 123 }), []);
});

test('Write tool: rejects absurdly long file_path (prompt injection guard)', () => {
  const huge = 'a'.repeat(2000) + '.png';
  assert.deepStrictEqual(extractImagePathsFromToolInput('Write', { file_path: huge }), []);
});

test('Edit tool: same field as Write', () => {
  const out = extractImagePathsFromToolInput('Edit', { file_path: './chart.jpg' });
  assert.deepStrictEqual(out, ['./chart.jpg']);
});

// ── extractImagePathsFromToolInput: Bash ────────────────────────

test('Bash tool: extracts Puppeteer-style output path', () => {
  const cmd = 'node -e "puppeteer.screenshot({path: \'results/chart.png\'})"';
  const out = extractImagePathsFromToolInput('Bash', { command: cmd });
  assert.ok(out.indexOf('results/chart.png') !== -1, `expected results/chart.png in ${JSON.stringify(out)}`);
});

test('Bash tool: extracts redirect-style output path', () => {
  const out = extractImagePathsFromToolInput('Bash', { command: 'convert input.svg output.png' });
  assert.ok(out.indexOf('input.svg') !== -1);
  assert.ok(out.indexOf('output.png') !== -1);
});

test('Bash tool: extracts --output=chart.jpg form', () => {
  const out = extractImagePathsFromToolInput('Bash', { command: 'tool --output=chart.jpg --verbose' });
  assert.ok(out.indexOf('chart.jpg') !== -1, `got ${JSON.stringify(out)}`);
});

test('Bash tool: extracts Windows absolute path', () => {
  const cmd = 'powershell -c "Save C:\\Users\\Foo\\Merlin\\results\\chart.png"';
  const out = extractImagePathsFromToolInput('Bash', { command: cmd });
  const hit = out.some((p) => p.toLowerCase().indexOf('chart.png') !== -1);
  assert.ok(hit, `expected a path containing chart.png, got ${JSON.stringify(out)}`);
});

test('Bash tool: ignores missing command', () => {
  assert.deepStrictEqual(extractImagePathsFromToolInput('Bash', {}), []);
});

test('Bash tool: a pure `ls` call produces nothing', () => {
  assert.deepStrictEqual(extractImagePathsFromToolInput('Bash', { command: 'ls -la' }), []);
});

// ── extractImagePathsFromToolInput: unknown tools ───────────────

test('Read tool: returns empty (path is input, not output)', () => {
  // We intentionally do NOT track Read — the file existed before the turn,
  // so it's not an artifact the model "produced."
  assert.deepStrictEqual(extractImagePathsFromToolInput('Read', { file_path: 'foo.png' }), []);
});

test('unknown MCP tool: returns empty', () => {
  assert.deepStrictEqual(extractImagePathsFromToolInput('mcp__merlin__image', { foo: 'bar.png' }), []);
});

// ── normalizeImagePathForMerlinUrl ──────────────────────────────

test('normalize: relative path passes through', () => {
  assert.strictEqual(normalizeImagePathForMerlinUrl('results/chart.png'), 'results/chart.png');
});

test('normalize: ./ prefix stripped', () => {
  assert.strictEqual(normalizeImagePathForMerlinUrl('./chart.png'), 'chart.png');
});

test('normalize: backslashes converted to forward slashes', () => {
  assert.strictEqual(normalizeImagePathForMerlinUrl('results\\chart.png'), 'results/chart.png');
});

test('normalize: Windows absolute path inside Merlin workspace → workspace-relative', () => {
  assert.strictEqual(
    normalizeImagePathForMerlinUrl('C:\\Users\\Ryan\\Documents\\Merlin\\results\\chart.png'),
    'results/chart.png',
  );
});

test('normalize: Windows absolute path OUTSIDE Merlin workspace → rejected', () => {
  assert.strictEqual(normalizeImagePathForMerlinUrl('C:\\Windows\\Temp\\chart.png'), '');
});

test('normalize: POSIX absolute path → rejected (can\'t serve from /)', () => {
  assert.strictEqual(normalizeImagePathForMerlinUrl('/tmp/chart.png'), '');
});

test('normalize: parent-dir escape → rejected', () => {
  assert.strictEqual(normalizeImagePathForMerlinUrl('../outside.png'), '');
});

test('normalize: empty and non-string input → ""', () => {
  assert.strictEqual(normalizeImagePathForMerlinUrl(''), '');
  assert.strictEqual(normalizeImagePathForMerlinUrl(null), '');
  assert.strictEqual(normalizeImagePathForMerlinUrl(undefined), '');
  assert.strictEqual(normalizeImagePathForMerlinUrl(42), '');
});

// ── uniqueByBasename ────────────────────────────────────────────

test('uniqueByBasename: dedupes same file under different path spellings', () => {
  const out = uniqueByBasename(['./chart.png', 'chart.png', 'results/chart.png']);
  assert.strictEqual(out.length, 1);
  assert.strictEqual(out[0], './chart.png'); // first-seen wins
});

test('uniqueByBasename: preserves distinct files', () => {
  const out = uniqueByBasename(['a.png', 'b.png', 'c.png']);
  assert.deepStrictEqual(out, ['a.png', 'b.png', 'c.png']);
});

test('uniqueByBasename: tolerates garbage entries', () => {
  const out = uniqueByBasename([null, '', 'chart.png', undefined, 'chart.png']);
  assert.deepStrictEqual(out, ['chart.png']);
});

// ── bubbleAlreadyReferences ─────────────────────────────────────

test('bubbleAlreadyReferences: finds markdown image reference', () => {
  const html = '<p>See: <img src="merlin://results/chart.png" alt="chart"></p>';
  assert.ok(bubbleAlreadyReferences(html, 'chart.png'));
});

test('bubbleAlreadyReferences: finds bare path mention', () => {
  const html = '<p>Wrote the chart to results/chart.png.</p>';
  assert.ok(bubbleAlreadyReferences(html, 'chart.png'));
});

test('bubbleAlreadyReferences: returns false when unreferenced', () => {
  const html = '<p>There it is — rendering works perfectly in Merlin.</p>';
  assert.ok(!bubbleAlreadyReferences(html, 'chart.png'));
});

test('bubbleAlreadyReferences: case-insensitive', () => {
  assert.ok(bubbleAlreadyReferences('<img src="CHART.PNG">', 'chart.png'));
});

test('bubbleAlreadyReferences: tolerates non-string input', () => {
  assert.ok(!bubbleAlreadyReferences(null, 'chart.png'));
  assert.ok(!bubbleAlreadyReferences('<html/>', ''));
});

// ── Integration: the ROAS chart scenario that motivated this code ──

test('integration: ROAS chart turn — Puppeteer Bash + no reference in final text → artifact surfaced', () => {
  // Simulate the 2026-04-20 user-reported turn: model writes SVG, runs
  // Puppeteer via Bash to produce PNG, then finalizes with text that does
  // NOT mention the chart path. We should still surface the PNG.
  const toolCalls = [
    { name: 'Write', input: { file_path: 'results/chart.svg', content: '<svg>...</svg>' } },
    { name: 'Bash', input: { command: 'node -e "puppeteer.screenshot({path: \'results/chart.png\'})"' } },
  ];
  const artifacts = [];
  for (const tc of toolCalls) {
    const paths = extractImagePathsFromToolInput(tc.name, tc.input);
    for (const p of paths) if (artifacts.indexOf(p) === -1) artifacts.push(p);
  }
  // Final text that forgets to embed the chart.
  const finalHtml = '<p>There it is — rendering works perfectly in Merlin.</p>';
  const unique = uniqueByBasename(artifacts);
  const toEmbed = unique.filter((p) => {
    const base = p.split(/[\\\/]/).pop();
    return !bubbleAlreadyReferences(finalHtml, base) && normalizeImagePathForMerlinUrl(p) !== '';
  });
  // Expect at least the PNG to be surfaced (the SVG may or may not depending
  // on the regex's greediness; what matters is that the final PNG lands).
  const pngs = toEmbed.filter((p) => p.toLowerCase().endsWith('.png'));
  assert.ok(pngs.length >= 1, `expected PNG to be surfaced, got ${JSON.stringify(toEmbed)}`);
});

test('integration: model that DID embed the chart → nothing surfaced (no duplicate)', () => {
  const toolCalls = [
    { name: 'Write', input: { file_path: 'results/chart.png', content: 'binary' } },
  ];
  const artifacts = [];
  for (const tc of toolCalls) {
    const paths = extractImagePathsFromToolInput(tc.name, tc.input);
    for (const p of paths) if (artifacts.indexOf(p) === -1) artifacts.push(p);
  }
  const finalHtml = '<p>Here\'s the chart: <img src="merlin://results/chart.png"></p>';
  const unique = uniqueByBasename(artifacts);
  const toEmbed = unique.filter((p) => {
    const base = p.split(/[\\\/]/).pop();
    return !bubbleAlreadyReferences(finalHtml, base);
  });
  assert.strictEqual(toEmbed.length, 0, `expected no extra surface, got ${JSON.stringify(toEmbed)}`);
});

// ── Report ──────────────────────────────────────────────────────

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
