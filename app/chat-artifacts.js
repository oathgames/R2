// Pure helpers for the chat renderer's "turn image artifacts" feature.
//
// The renderer scans every tool_use that streams by in a turn, pulls out any
// file paths with image extensions, and on turn-end appends any that weren't
// referenced in the final bubble text as inline <img>s. The logic is pulled
// out here so it's testable in Node without the DOM.
//
// See the REGRESSION GUARD blocks in app/renderer.js for the full why.
//
// Dual-module shim: loaded as a global (window.MerlinChatArtifacts) in the
// renderer, and as a CommonJS module in the Node test harness.

(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory();
  } else {
    root.MerlinChatArtifacts = factory();
  }
}(typeof self !== 'undefined' ? self : this, function () {
  const IMAGE_EXT_RE = /\.(png|jpg|jpeg|gif|webp|svg)$/i;

  // Pull image-extension paths out of a tool_use `input` object. Different
  // tools name the path field differently; we normalize them here. Accepts
  // unknown tools gracefully (returns []).
  //
  // Recognized shapes:
  //   Write / Edit / NotebookEdit → input.file_path
  //   Bash                         → input.command (free-form; regex-scanned)
  function extractImagePathsFromToolInput(toolName, input) {
    const out = [];
    if (!input || typeof input !== 'object') return out;
    const push = (p) => {
      if (typeof p !== 'string') return;
      const trimmed = p.trim().replace(/^['"]|['"]$/g, '');
      if (trimmed.length === 0 || trimmed.length > 1024) return;
      if (IMAGE_EXT_RE.test(trimmed)) out.push(trimmed);
    };
    if (toolName === 'Write' || toolName === 'Edit' || toolName === 'NotebookEdit') {
      push(input.file_path);
    } else if (toolName === 'Bash') {
      const cmd = String(input.command || '');
      // Capture image paths in any of three shapes a Bash command can
      // plausibly name them. The regex is intentionally permissive — a
      // false positive just means one extra inline image; a false negative
      // means the chart disappears.
      //   group 1: Windows absolute (`C:\path\to\foo.png`, quoted or bare)
      //   group 2: quoted relative  (`"./foo.png"` or `'results/foo.png'`)
      //   group 3: unquoted token   (whitespace/`=`-delimited, no spaces)
      const re = /["']?([A-Za-z]:[\\\/][^"'\s<>|]+?\.(?:png|jpg|jpeg|gif|webp|svg))["']?|["']((?:\.{1,2}[\\\/])?[\w\-.\\\/]+?\.(?:png|jpg|jpeg|gif|webp|svg))["']|(?:^|[\s=])((?:\.{1,2}[\\\/])?[\w\-.\\\/]+?\.(?:png|jpg|jpeg|gif|webp|svg))/gi;
      let m;
      while ((m = re.exec(cmd)) !== null) push(m[1] || m[2] || m[3]);
    }
    return out;
  }

  // Normalize a path to the form the `merlin://` protocol handler expects:
  // forward slashes, no leading `./`, workspace-relative. Returns '' for
  // shapes that can't be safely served (absolute paths outside the Merlin
  // workspace, parent-dir escapes).
  function normalizeImagePathForMerlinUrl(raw) {
    if (typeof raw !== 'string') return '';
    let p = raw.trim().replace(/\\/g, '/').replace(/^\.\//, '');
    if (p.startsWith('/')) return '';
    const drv = /^[A-Za-z]:\//.exec(p);
    if (drv) {
      const idx = p.toLowerCase().indexOf('/merlin/');
      if (idx < 0) return '';
      p = p.slice(idx + '/merlin/'.length);
    }
    if (p.startsWith('../') || p === '..' || p.length === 0) return '';
    return p;
  }

  // Deduplicate a list of paths by basename, preserving first-seen order.
  // Used so the "trailing attachments" block never shows the same file twice
  // under different path spellings (e.g. `./chart.png` and `chart.png`).
  function uniqueByBasename(paths) {
    const seen = new Set();
    const out = [];
    for (const p of paths || []) {
      if (typeof p !== 'string' || p.length === 0) continue;
      const base = p.split(/[\\\/]/).pop();
      if (!base || seen.has(base)) continue;
      seen.add(base);
      out.push(p);
    }
    return out;
  }

  // Does `haystack` already reference `filename` (by basename)? Used to skip
  // auto-embedding an image the model already embedded via markdown. Simple
  // case-insensitive substring — markdown-image syntax, bare paths, and
  // <img src="..."> all contain the basename.
  function bubbleAlreadyReferences(haystack, filename) {
    if (typeof haystack !== 'string' || typeof filename !== 'string') return false;
    if (filename.length === 0) return false;
    return haystack.toLowerCase().indexOf(filename.toLowerCase()) !== -1;
  }

  return {
    IMAGE_EXT_RE,
    extractImagePathsFromToolInput,
    normalizeImagePathForMerlinUrl,
    uniqueByBasename,
    bubbleAlreadyReferences,
  };
}));
