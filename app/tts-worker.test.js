// Tests for tts-worker.js input validation.
//
// The worker is normally forked as an Electron utility process. These
// tests import it as a plain module — the parentPort guard added in
// REGRESSION GUARD (2026-04-19) keeps the message-handler registration
// off unless parentPort exists, so requiring the module here is safe.

'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');

const { validateSpeechText, MAX_SPEECH_TEXT_CHARS, isOnnxBackendError } = require('./tts-worker');

test('validateSpeechText accepts typical sentence', () => {
  const r = validateSpeechText('Hello, world.');
  assert.equal(r.ok, true);
  assert.equal(r.text, 'Hello, world.');
});

test('validateSpeechText rejects null', () => {
  const r = validateSpeechText(null);
  assert.equal(r.ok, false);
  assert.match(r.error, /No speech text/);
});

test('validateSpeechText rejects undefined', () => {
  const r = validateSpeechText(undefined);
  assert.equal(r.ok, false);
  assert.match(r.error, /No speech text/);
});

test('validateSpeechText rejects numeric input', () => {
  const r = validateSpeechText(42);
  assert.equal(r.ok, false);
  assert.match(r.error, /must be a string/);
});

test('validateSpeechText rejects boolean input', () => {
  const r = validateSpeechText(true);
  assert.equal(r.ok, false);
  assert.match(r.error, /must be a string/);
});

test('validateSpeechText rejects object input', () => {
  const r = validateSpeechText({ sentence: 'hi' });
  assert.equal(r.ok, false);
  assert.match(r.error, /must be a string/);
});

test('validateSpeechText rejects empty string', () => {
  const r = validateSpeechText('');
  assert.equal(r.ok, false);
  assert.match(r.error, /empty/);
});

test('validateSpeechText rejects whitespace-only string', () => {
  const r = validateSpeechText('   \n\t  ');
  assert.equal(r.ok, false);
  assert.match(r.error, /empty/);
});

test('validateSpeechText rejects text over the length ceiling', () => {
  const oversized = 'a'.repeat(MAX_SPEECH_TEXT_CHARS + 1);
  const r = validateSpeechText(oversized);
  assert.equal(r.ok, false);
  assert.match(r.error, /too long/);
  // Error should NOT echo the full payload back — it'd defeat the point
  // of guarding on length.
  assert.ok(!r.error.includes(oversized));
});

test('validateSpeechText accepts text exactly at the length ceiling', () => {
  const atLimit = 'a'.repeat(MAX_SPEECH_TEXT_CHARS);
  const r = validateSpeechText(atLimit);
  assert.equal(r.ok, true);
  assert.equal(r.text.length, MAX_SPEECH_TEXT_CHARS);
});

test('validateSpeechText NFC-normalizes decomposed Unicode', () => {
  // "é" composed vs "e" + combining acute. Same visible character, two
  // different code-point sequences. NFC collapses to the composed form.
  const decomposed = 'caf\u0065\u0301'; // "café" with combining mark
  const r = validateSpeechText(decomposed);
  assert.equal(r.ok, true);
  assert.equal(r.text, 'café');
  // The normalized string should be shorter than the decomposed input.
  assert.ok(r.text.length < decomposed.length);
});

// ── isOnnxBackendError ───────────────────────────────────────────
// Broad by design: matches the full class of ONNX-runtime backend errors
// so the CPU fallback fires on ANY driver-level failure, not just the
// specific ConvTranspose case that prompted the incident. See REGRESSION
// GUARD (2026-04-19, ConvTranspose incident) in tts-worker.js.

test('isOnnxBackendError catches the exact ConvTranspose signature from the incident', () => {
  const e = new Error("Non-zero status code returned while running ConvTranspose node. Name:'/encoder/N.1/pool/ConvTranspose' Status Message: D:\\a\\_work\\1\\s\\onnxruntime...");
  assert.equal(isOnnxBackendError(e), true);
});

test('isOnnxBackendError catches other ONNX ops (Conv, GEMM, LayerNorm)', () => {
  for (const op of ['Conv', 'GEMM', 'LayerNormalization', 'Resize']) {
    const e = new Error(`Non-zero status code returned while running ${op} node.`);
    assert.equal(isOnnxBackendError(e), true, `expected match for ${op}`);
  }
});

test('isOnnxBackendError catches onnxruntime path signature alone', () => {
  const e = new Error('some weird failure in onnxruntime/core/providers/dml');
  assert.equal(isOnnxBackendError(e), true);
});

test('isOnnxBackendError catches DirectML / CoreML prefixed messages', () => {
  assert.equal(isOnnxBackendError(new Error('DirectML device lost')), true);
  assert.equal(isOnnxBackendError(new Error('CoreML execution provider error')), true);
  assert.equal(isOnnxBackendError(new Error('DML: GPU hung')), true);
});

test('isOnnxBackendError accepts plain strings, not just Errors', () => {
  assert.equal(isOnnxBackendError('Non-zero status code returned while running Conv'), true);
});

test('isOnnxBackendError ignores unrelated errors', () => {
  assert.equal(isOnnxBackendError(new Error('phonemizer stalled')), false);
  assert.equal(isOnnxBackendError(new Error('bad voice')), false);
  assert.equal(isOnnxBackendError(null), false);
  assert.equal(isOnnxBackendError(undefined), false);
  assert.equal(isOnnxBackendError(''), false);
});
