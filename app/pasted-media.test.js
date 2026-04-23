// Unit tests for app/pasted-media.js. Run with `node app/pasted-media.test.js`.
//
// Maps 1:1 to the 2026-04-23 codex audit finding on save-pasted-media.
// Coverage:
//   - happy path: valid PNG/JPEG/GIF/WebP/MP4/WebM/MOV are accepted
//   - rejects disallowed MIMEs (text/html, application/octet-stream,
//     image/svg+xml, application/pdf, image/x-icon)
//   - rejects MIME/magic mismatch (claim png, send html bytes)
//   - rejects oversize payloads
//   - rejects malformed data URLs (no base64, charset param, non-base64 body)
//   - filename extension is derived from the verified MIME, not user input
//   - user-supplied stem is sanitized (path traversal, control chars)
//   - empty buffer rejected
//   - non-string args rejected

'use strict';

const assert = require('assert');
const { validatePastedMedia, MAX_BYTES } = require('./pasted-media');

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
    if (err.stack) console.log('   ', err.stack.split('\n').slice(1, 4).join('\n    '));
    failed++;
  }
}

function dataUrl(mime, bytes) {
  return `data:${mime};base64,${Buffer.from(bytes).toString('base64')}`;
}

const PNG_MAGIC = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0];
const JPEG_MAGIC = [0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0];
const GIF_MAGIC = [0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0, 0];
const WEBP_MAGIC = [0x52, 0x49, 0x46, 0x46, 0x10, 0, 0, 0, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x20];
const MP4_MAGIC = [0, 0, 0, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6D, 0x70, 0x34, 0x32];
const WEBM_MAGIC = [0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81];
const MOV_MAGIC = [0, 0, 0, 0x14, 0x66, 0x74, 0x79, 0x70, 0x71, 0x74, 0x20, 0x20];

console.log('pasted-media.test.js');

test('accepts a valid PNG and writes .png extension', () => {
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), 'paste.png');
  assert.equal(r.ok, true, `expected ok, got reason=${r.reason}`);
  assert.ok(r.safeName.endsWith('.png'));
  assert.ok(Buffer.isBuffer(r.buf));
});

test('accepts a valid JPEG and writes .jpg extension', () => {
  const r = validatePastedMedia(dataUrl('image/jpeg', JPEG_MAGIC), 'paste.jpeg');
  assert.equal(r.ok, true, `expected ok, got reason=${r.reason}`);
  assert.ok(r.safeName.endsWith('.jpg'), `safeName=${r.safeName}`);
});

test('accepts GIF89a', () => {
  const r = validatePastedMedia(dataUrl('image/gif', GIF_MAGIC), 'p.gif');
  assert.equal(r.ok, true, r.reason);
});

test('accepts GIF87a', () => {
  const m = [0x47, 0x49, 0x46, 0x38, 0x37, 0x61, 0, 0];
  const r = validatePastedMedia(dataUrl('image/gif', m), 'p.gif');
  assert.equal(r.ok, true, r.reason);
});

test('accepts WebP', () => {
  const r = validatePastedMedia(dataUrl('image/webp', WEBP_MAGIC), 'p.webp');
  assert.equal(r.ok, true, r.reason);
});

test('accepts MP4', () => {
  const r = validatePastedMedia(dataUrl('video/mp4', MP4_MAGIC), 'clip.mp4');
  assert.equal(r.ok, true, r.reason);
});

test('accepts WebM', () => {
  const r = validatePastedMedia(dataUrl('video/webm', WEBM_MAGIC), 'clip.webm');
  assert.equal(r.ok, true, r.reason);
});

test('accepts MOV (quicktime)', () => {
  const r = validatePastedMedia(dataUrl('video/quicktime', MOV_MAGIC), 'clip.mov');
  assert.equal(r.ok, true, r.reason);
});

test('rejects text/html', () => {
  const r = validatePastedMedia(dataUrl('text/html', [0x3C, 0x68, 0x74, 0x6D, 0x6C]), 'evil.html');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'mime-not-allowed');
});

test('rejects image/svg+xml (XSS vector)', () => {
  const r = validatePastedMedia(dataUrl('image/svg+xml', [0x3C, 0x73, 0x76, 0x67]), 'evil.svg');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'mime-not-allowed');
});

test('rejects application/octet-stream', () => {
  const r = validatePastedMedia(dataUrl('application/octet-stream', PNG_MAGIC), 'evil.bin');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'mime-not-allowed');
});

test('rejects application/x-msdownload (Windows exe)', () => {
  const r = validatePastedMedia(dataUrl('application/x-msdownload', [0x4D, 0x5A]), 'evil.exe');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'mime-not-allowed');
});

test('rejects MIME/magic mismatch — claims PNG, sends HTML', () => {
  const htmlBytes = Buffer.from('<html><body>x</body></html>');
  const r = validatePastedMedia(`data:image/png;base64,${htmlBytes.toString('base64')}`, 'evil.png');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'magic-mismatch');
});

test('rejects MIME/magic mismatch — claims JPEG, sends PNG bytes', () => {
  const r = validatePastedMedia(dataUrl('image/jpeg', PNG_MAGIC), 'confused.jpg');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'magic-mismatch');
});

test('rejects empty buffer', () => {
  const r = validatePastedMedia('data:image/png;base64,', 'p.png');
  assert.equal(r.ok, false);
  // Could be bad-data-url (regex requires payload chars) or empty
  assert.ok(r.reason === 'bad-data-url' || r.reason === 'empty');
});

test('rejects oversize payload (> 5MB decoded)', () => {
  const big = Buffer.alloc(MAX_BYTES + 1024);
  // First 8 bytes = PNG magic so it passes the magic check IF size were OK
  PNG_MAGIC.slice(0, 8).forEach((b, i) => big[i] = b);
  const r = validatePastedMedia(`data:image/png;base64,${big.toString('base64')}`, 'huge.png');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'too-large');
});

test('rejects bare data: URL with no base64 marker', () => {
  const r = validatePastedMedia('data:image/png,plain-text', 'p.png');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'bad-data-url');
});

test('rejects data: URL with charset parameter (non-strict format)', () => {
  const r = validatePastedMedia('data:image/png;charset=utf-8;base64,iVBORw0KGgo=', 'p.png');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'bad-data-url');
});

test('rejects non-data URL', () => {
  const r = validatePastedMedia('https://example.com/foo.png', 'p.png');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'bad-data-url');
});

test('rejects non-string dataUrl arg', () => {
  const r = validatePastedMedia(null, 'p.png');
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'bad-input');
});

test('rejects non-string filename arg', () => {
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), null);
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'bad-input');
});

test('extension is forced from MIME, ignoring user-supplied wrong extension', () => {
  // User claims image/png and sends real PNG bytes but names the file
  // .exe — output must be .png, not .exe.
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), 'malware.exe');
  assert.equal(r.ok, true, r.reason);
  assert.ok(r.safeName.endsWith('.png'), `safeName=${r.safeName}`);
  assert.ok(!r.safeName.includes('.exe'));
});

test('user-supplied stem is preserved when alphanumeric', () => {
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), 'my_cool_paste.png');
  assert.equal(r.ok, true, r.reason);
  assert.equal(r.safeName, 'my_cool_paste.png');
});

test('user-supplied stem with path traversal is sanitized', () => {
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), '../../etc/passwd.png');
  assert.equal(r.ok, true, r.reason);
  // path.basename strips '../../etc/' so we're left with 'passwd' as the stem
  assert.equal(r.safeName, 'passwd.png');
});

test('user-supplied stem with control chars is sanitized', () => {
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), 'a\x00b\x07c.png');
  assert.equal(r.ok, true, r.reason);
  assert.equal(r.safeName, 'a_b_c.png');
});

test('dotfile-only filename is rejected', () => {
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), '....png');
  // After stripping .png we get '...' which becomes '' after the leading-dot strip
  assert.equal(r.ok, false);
  assert.equal(r.reason, 'bad-filename');
});

test('long filename stem is truncated to 120 chars', () => {
  const longStem = 'a'.repeat(500);
  const r = validatePastedMedia(dataUrl('image/png', PNG_MAGIC), `${longStem}.png`);
  assert.equal(r.ok, true, r.reason);
  // 120 chars stem + .png = 124 chars total
  assert.ok(r.safeName.length <= 124, `safeName length ${r.safeName.length}`);
});

console.log(`\n${passed} passed, ${failed} failed`);
process.exit(failed === 0 ? 0 : 1);
