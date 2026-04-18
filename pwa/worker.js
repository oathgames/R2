// Merlin PWA — Cloudflare Worker
//
// Serves the PWA shell (index.html, pwa.js, sw.js, style, manifest).
// Sets a strict Content-Security-Policy that pins connect-src to the relay
// origin — a compromised script cannot exfiltrate to arbitrary hosts.
//
// Security:
//   - CSP: default-src 'self'; script-src is 'self' only (no inline). If a
//     future change needs inline, prefer a nonce — do NOT add
//     'unsafe-inline'. script-src 'unsafe-inline' defeats CSP's primary
//     value (see incident history for landing/ v1.0.0 in CLAUDE.md).
//   - connect-src pins to the relay HTTPS + WSS origin. Blocks any drive-by
//     fetch to third-party hosts from injected JS.
//   - frame-ancestors 'none' blocks clickjacking.
//   - Strict-Transport-Security with preload — the PWA is HTTPS-only.
//   - sw.js served under the root so it can control the whole origin
//     scope (browsers enforce scope ≤ SW path).

import HTML from './index.html';
import JS from './pwa.js';
import SW from './sw.js';
import CSS from './style.css';
import MANIFEST from './manifest.json';

const RELAY_ORIGIN = 'https://relay.merlingotme.com';
const RELAY_WS    = 'wss://relay.merlingotme.com';

const CSP = [
  "default-src 'self'",
  "script-src 'self'",
  // Inline styles: the page uses a tiny inline style attribute for
  // question-bubble spacing. Allow 'unsafe-inline' in style-src ONLY, not
  // script-src. Google Fonts also requires loading stylesheets from its
  // origin.
  "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
  "font-src  'self' https://fonts.gstatic.com",
  "img-src   'self' data:",
  `connect-src 'self' ${RELAY_ORIGIN} ${RELAY_WS}`,
  "manifest-src 'self'",
  "worker-src 'self'",
  "frame-ancestors 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join('; ');

const SEC_HEADERS = {
  'Content-Security-Policy': CSP,
  'Strict-Transport-Security': 'max-age=63072000; includeSubDomains; preload',
  'X-Content-Type-Options': 'nosniff',
  'X-Frame-Options': 'DENY',
  'Referrer-Policy': 'no-referrer',
  'Permissions-Policy': 'geolocation=(), microphone=(), camera=(), payment=()',
};

function respond(body, contentType, cacheControl = 'public, max-age=3600', extra = {}) {
  return new Response(body, {
    headers: {
      'Content-Type': contentType,
      'Cache-Control': cacheControl,
      ...SEC_HEADERS,
      ...extra,
    },
  });
}

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const path = url.pathname;

    if (path === '/pwa.js') {
      return respond(JS, 'application/javascript;charset=UTF-8');
    }
    if (path === '/sw.js') {
      // Service worker scope = directory of the SW file. Must be served at
      // root for it to control the whole origin. no-store avoids staleness
      // of a controller that can outlive any page-level cache.
      return respond(SW, 'application/javascript;charset=UTF-8', 'no-store', {
        'Service-Worker-Allowed': '/',
      });
    }
    if (path === '/style.css') {
      return respond(CSS, 'text/css;charset=UTF-8');
    }
    if (path === '/manifest.json') {
      return respond(MANIFEST, 'application/json', 'public, max-age=86400');
    }

    return respond(HTML, 'text/html;charset=UTF-8');
  },
};
