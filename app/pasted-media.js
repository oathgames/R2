// Validation for ipcMain('save-pasted-media').
//
// REGRESSION GUARD (2026-04-23, codex audit session/security-quick-wins):
// Extracted from main.js so the logic is unit-testable without booting
// Electron. Three layers of defense; DO NOT loosen any one in isolation:
//
//   1. MIME allowlist — only image/{png,jpeg,gif,webp} and
//      video/{mp4,webm,quicktime}. Nothing executable, nothing HTML.
//      Claiming a disallowed MIME (text/html, application/*, svg+xml)
//      is an immediate rejection.
//   2. Magic-byte check — the decoded bytes MUST begin with the file
//      signature for the CLAIMED MIME. Defeats "rename .exe to .png"
//      and "claim image/png but send HTML/SVG payload" attacks.
//   3. Trusted extension — the filename extension written to disk is
//      derived from the VERIFIED MIME, not from the user-supplied
//      filename. The user's chosen stem is preserved (and sanitized
//      independently) but the extension is canonical.
//
// Size cap is also enforced here: 5 MB of decoded bytes. The preload
// caps the incoming dataUrl string at 5,000,000 chars which decodes to
// ~3.75 MB, so 5 MB after decode is a generous ceiling that still bounds
// disk growth per paste and matches the preload assertion's intent.

'use strict';

const path = require('node:path');

const MIME_ALLOWLIST = {
  'image/png':       { ext: 'png',  check: (b) => b.length >= 8 && b[0] === 0x89 && b[1] === 0x50 && b[2] === 0x4E && b[3] === 0x47 && b[4] === 0x0D && b[5] === 0x0A && b[6] === 0x1A && b[7] === 0x0A },
  'image/jpeg':      { ext: 'jpg',  check: (b) => b.length >= 3 && b[0] === 0xFF && b[1] === 0xD8 && b[2] === 0xFF },
  'image/gif':       { ext: 'gif',  check: (b) => b.length >= 6 && b[0] === 0x47 && b[1] === 0x49 && b[2] === 0x46 && b[3] === 0x38 && (b[4] === 0x37 || b[4] === 0x39) && b[5] === 0x61 },
  'image/webp':      { ext: 'webp', check: (b) => b.length >= 12 && b[0] === 0x52 && b[1] === 0x49 && b[2] === 0x46 && b[3] === 0x46 && b[8] === 0x57 && b[9] === 0x45 && b[10] === 0x42 && b[11] === 0x50 },
  'video/mp4':       { ext: 'mp4',  check: (b) => b.length >= 12 && b[4] === 0x66 && b[5] === 0x74 && b[6] === 0x79 && b[7] === 0x70 },
  'video/webm':      { ext: 'webm', check: (b) => b.length >= 4 && b[0] === 0x1A && b[1] === 0x45 && b[2] === 0xDF && b[3] === 0xA3 },
  'video/quicktime': { ext: 'mov',  check: (b) => b.length >= 12 && b[4] === 0x66 && b[5] === 0x74 && b[6] === 0x79 && b[7] === 0x70 },
};

const MAX_BYTES = 5 * 1024 * 1024;

// Strict data URL pattern — no `;charset=`, no URL-encoded variant, no
// parameters. `request.text().length > MAX_BYTES` style checks happen
// upstream; this regex just keeps the parse deterministic.
const DATA_URL_RE = /^data:([a-z]+\/[a-z0-9.+\-]+);base64,([A-Za-z0-9+/=]+)$/i;

function validatePastedMedia(dataUrl, filename) {
  if (typeof dataUrl !== 'string' || typeof filename !== 'string') {
    return { ok: false, reason: 'bad-input' };
  }
  const m = DATA_URL_RE.exec(dataUrl);
  if (!m) return { ok: false, reason: 'bad-data-url' };
  const mime = m[1].toLowerCase();
  const spec = MIME_ALLOWLIST[mime];
  if (!spec) return { ok: false, reason: 'mime-not-allowed' };

  let buf;
  try { buf = Buffer.from(m[2], 'base64'); } catch { return { ok: false, reason: 'decode-failed' }; }
  if (!buf.length) return { ok: false, reason: 'empty' };
  if (buf.length > MAX_BYTES) return { ok: false, reason: 'too-large' };
  if (!spec.check(buf)) return { ok: false, reason: 'magic-mismatch' };

  const rawBase = path.basename(filename);
  const stemRaw = rawBase.replace(/\.[^.]+$/, '');
  const stem = stemRaw.replace(/[^a-zA-Z0-9._-]/g, '_').replace(/^\.+/, '').slice(0, 120);
  if (!stem) return { ok: false, reason: 'bad-filename' };
  return { ok: true, buf, safeName: `${stem}.${spec.ext}` };
}

module.exports = {
  validatePastedMedia,
  MIME_ALLOWLIST,
  MAX_BYTES,
};
