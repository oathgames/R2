// Merlin Captions — Hormozi-style word-level burn-in
//
// Burns word-by-word captions onto an existing video using the bundled
// ffmpeg + whisper-cli + ggml-small.en-q5_1 model. The toolchain ships
// inside the Electron installer (extraResources, .claude/tools/) — see
// release.yml's "Bundle voice tools" step. This module reuses the EXACT
// same binary-resolution path as `transcribeAudioImpl` in main.js so
// there is one source of truth for "where do the voice tools live."
//
// Pipeline:
//   1. Validate the input video (path, extension, size cap).
//   2. Resolve ffmpeg/whisper-cli/model via findVoiceTools.
//   3. Extract 16k mono PCM WAV (whisper.cpp's native input format —
//      avoids the model resampling on its own and shaves ~10% off
//      transcription time).
//   4. Run whisper-cli with `-ml 1 -oj` so each segment is a single
//      word with millisecond `offsets: {from, to}`.
//   5. Parse JSON → word list.
//   6. Build a libass `.ass` file: each word renders as its OWN
//      Dialogue line (3-word context bubble — previous + active + next),
//      Hormozi-style: bold sans-serif, ~14% of video height, yellow
//      active word, white context.
//   7. Burn with `ffmpeg -vf ass=...` (libass), CRF 18, audio passthrough.
//   8. Cleanup intermediates unless MERLIN_DEBUG_CAPTIONS=1.
//
// Public entry: burnCaptions({ videoPath, style, outputDir, appRoot,
// appInstall, abortSignal }) → { success, outputPath, wordCount,
// durationMs } | { error: <code>, errorDetail: <string> }.
//
// The ASS generator is a PURE function (no fs / no subprocess) so the
// test suite can exercise it against fixed whisper JSON fixtures without
// invoking the toolchain. That covers the bulk of the regression
// surface — the parts most likely to break (centisecond rounding, color
// codes, time formatting, edge cases in word windowing) are all tested.

'use strict';

const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

// ── Constants ────────────────────────────────────────────────

// Hard cap on input video size. Videos larger than this run too slowly
// to be a good in-app UX and risk OOM on whisper transcription. If a
// user truly needs to caption a feature-length video they can split it
// first — this tool is for ad creatives and short social videos, where
// 500 MB is already an order of magnitude above the typical 10-30 MB
// vertical 9:16.
const MAX_VIDEO_BYTES = 500 * 1024 * 1024;

// 10-minute wall-clock ceiling. Apple Silicon transcribes a 60s video
// in ~10s with small.en-q5_1; Windows AMD CPUs can be 5x slower and
// ffmpeg burn-in adds another ~real-time pass. 10 minutes is the
// safety net, NOT the expected wall time. Anything that approaches
// this is almost certainly stuck and we'd rather kill the process
// than leave the renderer waiting forever.
const SUBPROCESS_TIMEOUT_MS = 10 * 60 * 1000;

// Allowed input extensions. Other formats may technically work
// through ffmpeg, but we constrain the surface area: every shipped
// creative-gen path produces .mp4, every screen recording is .mov or
// .webm. Adding a new extension here is intentional, not accidental.
const ALLOWED_EXTENSIONS = new Set(['.mp4', '.mov', '.webm']);

// libass colors are in `&HAABBGGRR&` format (alpha + reverse RGB) —
// this is libass's native byte order and CANNOT be changed to RGB.
// AA=00 means fully opaque.
const COLOR_WHITE  = '&H00FFFFFF&';   // opaque white
const COLOR_YELLOW = '&H0000FFFF&';   // opaque yellow (Hormozi accent)
const COLOR_BLACK  = '&H00000000&';   // opaque black (outline + shadow)

// ── Public surface ───────────────────────────────────────────

module.exports = {
  burnCaptions,
  // Pure-function exports — used by app/captions.test.js to verify
  // ASS generation, word-window logic, and validation without spawning
  // the bundled toolchain.
  parseWhisperWordsJson,
  buildAssSubtitles,
  buildContextBubble,
  validateInput,
  findVoiceTools,
  formatAssTime,
  escapeAssText,
  MAX_VIDEO_BYTES,
  ALLOWED_EXTENSIONS,
  SUBPROCESS_TIMEOUT_MS,
  COLOR_WHITE,
  COLOR_YELLOW,
  COLOR_BLACK,
};

// ── Voice toolchain resolution ────────────────────────────────
//
// Mirrors the `findTool` closure in main.js's transcribeAudioImpl.
// Kept as a separate function (rather than `require()`-ing into
// main.js) so this module is testable in isolation — the test passes
// a synthetic { appInstall, appRoot, fs } and asserts the resolution
// order is install-first, workspace-second.
function findVoiceTools({ appInstall, appRoot, fs: fsLike, path: pathLike, isWin } = {}) {
  const _fs = fsLike || fs;
  const _path = pathLike || path;
  const _isWin = typeof isWin === 'boolean' ? isWin : (process.platform === 'win32');
  const find = (name) => {
    if (appInstall) {
      const installPath = _path.join(appInstall, '.claude', 'tools', name);
      try { if (_fs.existsSync(installPath)) return installPath; } catch {}
    }
    if (appRoot) {
      const workspacePath = _path.join(appRoot, '.claude', 'tools', name);
      try { if (_fs.existsSync(workspacePath)) return workspacePath; } catch {}
    }
    return null;
  };
  return {
    ffmpegPath: find(_isWin ? 'ffmpeg.exe' : 'ffmpeg'),
    ffprobePath: find(_isWin ? 'ffprobe.exe' : 'ffprobe'),
    whisperBin:  find(_isWin ? 'whisper-cli.exe' : 'whisper-cli'),
    modelPath:   find('ggml-small.en-q5_1.bin'),
  };
}

// ── Input validation ──────────────────────────────────────────

function validateInput({ videoPath, fs: fsLike } = {}) {
  const _fs = fsLike || fs;
  if (!videoPath || typeof videoPath !== 'string') {
    return { ok: false, error: 'captions:invalid-input', errorDetail: 'videoPath must be a non-empty string' };
  }
  if (!path.isAbsolute(videoPath)) {
    return { ok: false, error: 'captions:invalid-input', errorDetail: `videoPath must be absolute: ${videoPath}` };
  }
  const ext = path.extname(videoPath).toLowerCase();
  if (!ALLOWED_EXTENSIONS.has(ext)) {
    return { ok: false, error: 'captions:invalid-input', errorDetail: `Unsupported video extension: ${ext} (allowed: ${[...ALLOWED_EXTENSIONS].join(', ')})` };
  }
  let stat;
  try {
    stat = _fs.statSync(videoPath);
  } catch (e) {
    return { ok: false, error: 'captions:not-found', errorDetail: `Video not found: ${videoPath}` };
  }
  if (!stat.isFile()) {
    return { ok: false, error: 'captions:invalid-input', errorDetail: `Not a regular file: ${videoPath}` };
  }
  if (stat.size > MAX_VIDEO_BYTES) {
    return {
      ok: false,
      error: 'captions:too-large',
      errorDetail: `Video is ${(stat.size / 1024 / 1024).toFixed(0)}MB — limit is ${(MAX_VIDEO_BYTES / 1024 / 1024).toFixed(0)}MB. Split the video and caption parts separately.`,
    };
  }
  if (stat.size < 1024) {
    return { ok: false, error: 'captions:invalid-input', errorDetail: `Video is too small (${stat.size} bytes) — likely truncated or empty` };
  }
  return { ok: true, sizeBytes: stat.size };
}

// ── Whisper JSON → word list ─────────────────────────────────

// Whisper-cli with `-oj -ml 1` writes a JSON file that looks like:
//   {
//     "transcription": [
//       { "text": " Hello", "offsets": { "from": 0, "to": 320 }, ... },
//       { "text": " world", "offsets": { "from": 320, "to": 720 }, ... },
//       ...
//     ],
//     ...
//   }
// `-ml 1` means each segment is exactly one word. Offsets are in
// milliseconds. We return an array of `{ text, startMs, endMs }`.
//
// Defensive against:
//   - Whitespace / "[BLANK_AUDIO]" / silence markers.
//   - Punctuation-only segments (whisper sometimes emits commas/periods
//     as their own segment).
//   - Negative or non-monotonic offsets (broken model output —
//     clamp to >= previous endMs to keep ASS time stamps monotonic).
function parseWhisperWordsJson(json) {
  if (!json || typeof json !== 'object') return [];
  const segments = Array.isArray(json.transcription) ? json.transcription : [];
  const words = [];
  let prevEnd = 0;
  for (const seg of segments) {
    if (!seg || typeof seg !== 'object') continue;
    const raw = typeof seg.text === 'string' ? seg.text.trim() : '';
    if (!raw) continue;
    // Skip whisper diagnostic markers and pure punctuation.
    if (/^\[(?:[A-Z _]+|BLANK_AUDIO|MUSIC|APPLAUSE)\]$/i.test(raw)) continue;
    if (/^[^\p{L}\p{N}]+$/u.test(raw)) continue;
    const offsets = seg.offsets || {};
    let startMs = Number(offsets.from);
    let endMs = Number(offsets.to);
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs)) continue;
    if (startMs < 0) startMs = 0;
    if (endMs <= startMs) endMs = startMs + 200;     // floor: each word visible >= 200ms
    if (startMs < prevEnd) startMs = prevEnd;        // keep monotonic
    if (endMs <= startMs) endMs = startMs + 200;
    words.push({ text: raw, startMs: Math.round(startMs), endMs: Math.round(endMs) });
    prevEnd = endMs;
  }
  return words;
}

// ── ASS time format ──────────────────────────────────────────
//
// libass uses `H:MM:SS.cc` where `cc` is centiseconds (1/100s).
// Common mistake: shipping milliseconds instead — libass parses the
// first two digits as centiseconds and silently truncates everything
// after, so `0:00:01.500` becomes `0:00:01.50` (correct, 1.50s) but
// `0:00:01.1234` becomes `0:00:01.12` (silently wrong by 0.034s,
// which over a 60s video accumulates into visible drift). Round to
// centiseconds and emit exactly two digits.
function formatAssTime(ms) {
  if (!Number.isFinite(ms) || ms < 0) ms = 0;
  const totalCs = Math.round(ms / 10);    // centiseconds
  const cs = totalCs % 100;
  const totalS = Math.floor(totalCs / 100);
  const s = totalS % 60;
  const totalM = Math.floor(totalS / 60);
  const m = totalM % 60;
  const h = Math.floor(totalM / 60);
  const pad = (n, w) => String(n).padStart(w, '0');
  return `${h}:${pad(m, 2)}:${pad(s, 2)}.${pad(cs, 2)}`;
}

// ── ASS text escaping ────────────────────────────────────────
//
// libass treats `{`, `}`, `\`, `\n`, `\h` as control sequences. Any
// transcript that contains a literal `{` (e.g. someone reading code)
// would otherwise break the entire dialogue line. The escape rule is
// the standard one published in the libass docs.
function escapeAssText(s) {
  if (typeof s !== 'string') return '';
  return s
    .replace(/\\/g, '\\\\')
    .replace(/\{/g, '\\{')
    .replace(/\}/g, '\\}')
    // Newlines have no place in a single word — strip rather than
    // escape, otherwise stray \n in a transcript creates a literal
    // hard-break inside what should be one word.
    .replace(/[\r\n]+/g, ' ');
}

// ── Word-window logic ────────────────────────────────────────
//
// For each word at index i, the rendered "context bubble" is:
//
//     <prev word>  <ACTIVE WORD (yellow)>  <next word>
//
// Rationale: a single word on screen reads as a flash card and loses
// continuity (the user can't skim the upcoming word for context).
// A 3-word window matches Hormozi's actual ad style closely enough
// without overflowing the bottom-third safe area on vertical video.
// First and last words get a 2-word bubble (no prev / no next).
// Punctuation attached to the previous word's text (e.g. " world.")
// is preserved verbatim — it's the readable form.
//
// Returns an object with the formatted parts so the caller can
// assemble the dialogue text. Keeping this as a separate function
// means the test suite can verify the windowing rule in isolation.
function buildContextBubble(words, i) {
  const word = words[i];
  if (!word) return null;
  const prev = i > 0 ? words[i - 1] : null;
  const next = i < words.length - 1 ? words[i + 1] : null;
  return {
    prev: prev ? prev.text : '',
    active: word.text,
    next: next ? next.text : '',
  };
}

// ── ASS subtitle file builder ────────────────────────────────
//
// Produces a complete `.ass` file as a string. Pure function — no
// fs, no spawn. Used directly by the test suite.
//
// Style rules (Hormozi):
//   - PlayResX/Y default to 1080×1920 (vertical 9:16). Caller may
//     override after probing the source video. libass scales
//     positions/sizes proportionally if the actual video resolution
//     differs.
//   - Font: Arial Black (with Helvetica + sans-serif fallbacks).
//   - Size: ~14% of PlayResY (130 for 1920px) — punchy but not
//     overwhelming on a 9:16 phone screen.
//   - Outline: 4px black, Shadow: 2px black — readable on any
//     background (white text on white bg is the canonical caption
//     fail mode).
//   - Alignment 2 = bottom-center. MarginV puts the bottom edge of
//     the bubble at ~70% from the top, so it sits in the lower third
//     without overlapping the safe area at the bottom.
//   - Per-word color override via inline `{\c&...&}WORD{\r}` —
//     `\r` resets to the style default (white).
function buildAssSubtitles({ words, playResX = 1080, playResY = 1920, fontName = 'Arial Black', fontSize = null } = {}) {
  const safeWords = Array.isArray(words) ? words : [];
  const resY = Math.max(1, Math.round(playResY));
  const resX = Math.max(1, Math.round(playResX));
  const size = fontSize != null ? Math.round(fontSize) : Math.max(24, Math.round(resY * 0.072));
  const marginV = Math.round(resY * 0.30);

  const headerLines = [
    '[Script Info]',
    'ScriptType: v4.00+',
    'WrapStyle: 0',
    'ScaledBorderAndShadow: yes',
    `PlayResX: ${resX}`,
    `PlayResY: ${resY}`,
    'YCbCr Matrix: TV.709',
    '',
    '[V4+ Styles]',
    'Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding',
    // Bold=-1 (true), BorderStyle=1 (outline+shadow), Outline=4, Shadow=2, Alignment=2 (bottom-center).
    `Style: Default,${fontName},${size},${COLOR_WHITE},${COLOR_WHITE},${COLOR_BLACK},${COLOR_BLACK},-1,0,0,0,100,100,0,0,1,4,2,2,40,40,${marginV},1`,
    '',
    '[Events]',
    'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text',
  ];

  const eventLines = [];
  for (let i = 0; i < safeWords.length; i++) {
    const w = safeWords[i];
    if (!w) continue;
    const bubble = buildContextBubble(safeWords, i);
    const start = formatAssTime(w.startMs);
    const end = formatAssTime(w.endMs);
    // Build the dialogue text: prev (white) + active (yellow) + next (white).
    // We use `{\c&YYYY&}WORD{\r}` for the active word — \r resets to the
    // style's primary color (white) so subsequent words on the same line
    // stay default.
    const parts = [];
    if (bubble.prev) parts.push(escapeAssText(bubble.prev));
    parts.push(`{\\c${COLOR_YELLOW}}${escapeAssText(bubble.active)}{\\r}`);
    if (bubble.next) parts.push(escapeAssText(bubble.next));
    const text = parts.join(' ');
    // Format: Layer=0, Style=Default, Name="", margins=0 (use style default).
    eventLines.push(`Dialogue: 0,${start},${end},Default,,0,0,0,,${text}`);
  }

  return headerLines.concat(eventLines).join('\n') + '\n';
}

// ── Subprocess helpers ───────────────────────────────────────
//
// Each subprocess gets:
//   - `windowsHide: true` so we don't flash a console window on Windows.
//   - stderr captured (NOT redirected to ours — we need it for
//     classification, and inheriting stderr would leak ffmpeg's
//     verbose output to the Electron log).
//   - An AbortSignal listener that kills the process on cancel.
//   - A timeout that kills the process at SUBPROCESS_TIMEOUT_MS as a
//     last-resort safety net.
//
// The function rejects with a structured error: `{ code: 'captions:<step>',
// detail: <stderr|stdout snippet> }` so the caller can map to a friendly
// error code without parsing raw ffmpeg output.
function runSubprocess({ bin, args, step, abortSignal }) {
  return new Promise((resolve, reject) => {
    let killed = false;
    let timedOut = false;
    const child = spawn(bin, args, {
      windowsHide: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (d) => { stdout += d.toString(); });
    child.stderr.on('data', (d) => { stderr += d.toString(); });

    const timer = setTimeout(() => {
      timedOut = true;
      try { child.kill('SIGKILL'); } catch {}
    }, SUBPROCESS_TIMEOUT_MS);

    const onAbort = () => {
      killed = true;
      try { child.kill('SIGKILL'); } catch {}
    };
    if (abortSignal) {
      if (abortSignal.aborted) onAbort();
      else abortSignal.addEventListener('abort', onAbort, { once: true });
    }

    child.on('error', (err) => {
      clearTimeout(timer);
      if (abortSignal) abortSignal.removeEventListener('abort', onAbort);
      reject({
        code: `captions:${step}`,
        detail: String(err && err.message ? err.message : err),
      });
    });
    child.on('close', (code) => {
      clearTimeout(timer);
      if (abortSignal) abortSignal.removeEventListener('abort', onAbort);
      if (timedOut) {
        return reject({ code: `captions:${step}-timeout`, detail: `${step} exceeded ${SUBPROCESS_TIMEOUT_MS / 1000}s wall-clock limit` });
      }
      if (killed) {
        return reject({ code: `captions:cancelled`, detail: `${step} cancelled by caller` });
      }
      if (code === 0) {
        return resolve({ stdout, stderr });
      }
      reject({
        code: `captions:${step}`,
        detail: `${bin && path.basename(bin)} exit ${code}: ${stderr.slice(-500) || stdout.slice(-500) || '(no output)'}`,
      });
    });
  });
}

// ── ffprobe video resolution probe ───────────────────────────
//
// We default to 1080×1920. Probing the source lets us write
// resolution-matched ASS metadata so libass scales correctly on
// non-9:16 inputs (16:9 hero videos, 1:1 feed videos). Failure to
// probe is non-fatal — we keep the default and libass will scale
// proportionally either way.
async function probeVideoResolution({ ffprobePath, videoPath, abortSignal }) {
  if (!ffprobePath) return { width: 1080, height: 1920 };
  try {
    const { stdout } = await runSubprocess({
      bin: ffprobePath,
      args: ['-v', 'quiet', '-print_format', 'json', '-show_streams', '-select_streams', 'v:0', videoPath],
      step: 'probe',
      abortSignal,
    });
    const parsed = JSON.parse(stdout);
    const stream = Array.isArray(parsed.streams) && parsed.streams[0];
    const w = stream && Number(stream.width);
    const h = stream && Number(stream.height);
    if (Number.isFinite(w) && Number.isFinite(h) && w > 0 && h > 0) {
      return { width: w, height: h };
    }
  } catch (_) { /* fall through to default */ }
  return { width: 1080, height: 1920 };
}

// ── Main entry point ─────────────────────────────────────────

async function burnCaptions({ videoPath, style = 'hormozi', outputDir, appRoot, appInstall, abortSignal } = {}) {
  // Currently 'hormozi' is the only style. Future styles (e.g. 'minimal',
  // 'pop') would key off the `style` arg here. We accept the arg now to
  // pin the public contract — agents that pass `style: 'hormozi'`
  // explicitly won't break when more styles ship.
  if (style && style !== 'hormozi') {
    return { error: 'captions:invalid-input', errorDetail: `Unknown style "${style}". Supported: hormozi.` };
  }

  // 1. Validate video.
  const v = validateInput({ videoPath });
  if (!v.ok) return { error: v.error, errorDetail: v.errorDetail };

  // 2. Resolve toolchain.
  const tools = findVoiceTools({ appInstall, appRoot });
  const missing = [];
  if (!tools.ffmpegPath) missing.push('ffmpeg');
  if (!tools.whisperBin) missing.push('whisper-cli');
  if (!tools.modelPath) missing.push('ggml-small.en-q5_1 model');
  if (missing.length > 0) {
    return {
      error: 'captions:missing-tools',
      errorDetail: `Voice toolchain missing — ${missing.join(', ')} not found in install. Reinstall Merlin to restore.`,
    };
  }

  // 3. Resolve output directory.
  const ts = new Date().toISOString().replace(/[:.]/g, '').slice(0, 15);  // YYYYMMDDTHHMMSS
  // ts is ISO-without-punctuation: e.g. 20260428T143055. Stable + sortable.
  // We deliberately do NOT use Math.random() in the directory name — the
  // timestamp is precise enough that two captions runs in the same second
  // are an edge case (one tool call per minute is typical for ad creation).
  const resolvedOutputDir = outputDir
    ? path.resolve(outputDir)
    : path.join(appRoot || process.cwd(), 'results', `captioned_${ts}`);
  try {
    fs.mkdirSync(resolvedOutputDir, { recursive: true });
  } catch (e) {
    return { error: 'captions:io', errorDetail: `Failed to create output directory: ${e.message}` };
  }

  const audioPath = path.join(resolvedOutputDir, 'audio.wav');
  const transcriptPrefix = path.join(resolvedOutputDir, 'transcript');
  const transcriptJsonPath = `${transcriptPrefix}.json`;
  const subsPath = path.join(resolvedOutputDir, 'subs.ass');
  const finalPath = path.join(resolvedOutputDir, 'captioned.mp4');

  const t0 = Date.now();

  try {
    // 4. Extract 16k mono PCM. -y overwrites any stale file from a
    //    previous run in the same directory.
    await runSubprocess({
      bin: tools.ffmpegPath,
      args: ['-y', '-loglevel', 'error', '-i', videoPath, '-vn', '-acodec', 'pcm_s16le', '-ar', '16000', '-ac', '1', audioPath],
      step: 'audio-extract',
      abortSignal,
    });

    // 5. Transcribe with word-level timestamps.
    //    -ml 1 = max segment length 1 word.
    //    -oj   = output JSON.
    //    -of   = output file prefix (whisper-cli appends .json).
    //    -np   = no progress (cleaner stderr).
    await runSubprocess({
      bin: tools.whisperBin,
      args: ['-m', tools.modelPath, '-f', audioPath, '-l', 'en', '-ml', '1', '-oj', '-of', transcriptPrefix, '-np'],
      step: 'transcribe',
      abortSignal,
    });

    // 6. Read & parse the transcript JSON.
    let words;
    try {
      const raw = fs.readFileSync(transcriptJsonPath, 'utf8');
      const parsed = JSON.parse(raw);
      words = parseWhisperWordsJson(parsed);
    } catch (e) {
      return { error: 'captions:transcribe', errorDetail: `Failed to read transcript JSON: ${e.message}` };
    }
    if (words.length === 0) {
      return { error: 'captions:no-speech', errorDetail: 'No speech detected in the video — the audio may be silent, music-only, or not in English.' };
    }

    // 7. Probe the video for resolution-matched ASS metadata.
    const { width, height } = await probeVideoResolution({
      ffprobePath: tools.ffprobePath,
      videoPath,
      abortSignal,
    });

    // 8. Build the .ass file.
    const assContent = buildAssSubtitles({
      words,
      playResX: width,
      playResY: height,
    });
    try {
      fs.writeFileSync(subsPath, assContent, 'utf8');
    } catch (e) {
      return { error: 'captions:io', errorDetail: `Failed to write subtitle file: ${e.message}` };
    }

    // 9. Burn. Backslashes in Windows paths need escaping for the
    //    libass `ass=` filter — colons (drive letters) need escaping
    //    too, since `:` is libass's filter-option separator. The
    //    canonical workaround is to replace `\` with `/` and escape
    //    `:` as `\:`. ffmpeg accepts forward slashes on Windows.
    const filterPath = subsPath.replace(/\\/g, '/').replace(/:/g, '\\:');
    await runSubprocess({
      bin: tools.ffmpegPath,
      args: [
        '-y', '-loglevel', 'error',
        '-i', videoPath,
        '-vf', `ass=${filterPath}`,
        '-c:v', 'libx264', '-preset', 'fast', '-crf', '18',
        '-c:a', 'copy',
        finalPath,
      ],
      step: 'burn',
      abortSignal,
    });

    const durationMs = Date.now() - t0;

    // 10. Cleanup intermediates unless debug mode keeps them.
    if (process.env.MERLIN_DEBUG_CAPTIONS !== '1') {
      try { fs.unlinkSync(audioPath); } catch {}
      try { fs.unlinkSync(transcriptJsonPath); } catch {}
      try { fs.unlinkSync(subsPath); } catch {}
    }

    return {
      success: true,
      outputPath: finalPath,
      wordCount: words.length,
      durationMs,
    };
  } catch (err) {
    if (err && typeof err.code === 'string' && err.code.startsWith('captions:')) {
      return { error: err.code, errorDetail: err.detail || '' };
    }
    return { error: 'captions:internal', errorDetail: String(err && err.message ? err.message : err) };
  }
}

