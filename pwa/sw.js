// Merlin PWA Service Worker
//
// Two jobs:
//   1. Receive Web Push deliveries from the relay and surface them as
//      native notifications — this is the primary value prop (approval
//      alerts while roaming).
//   2. Focus an existing PWA window (or open one) when the user taps the
//      notification so they land directly on the pending approval.
//
// Deliberately NOT caching anything yet. A cache layer can be added later,
// but caching the shell before the auth flow is stable risks serving a
// stale pwa.js that disagrees with the Worker's CSP or the relay's
// protocol. Ship push first; cache second.
//
// Security:
//   - Push payloads from the relay are small JSON blobs with NO PII (see
//     durable.js firePushes — just `{t, id, title, body}`). We still
//     defensively validate shape and cap lengths before surfacing.
//   - notificationclick only routes to same-origin URLs, never arbitrary
//     ones from the payload.

const NOTIFICATION_TAG = 'merlin-approval';
const MAX_TITLE_LEN = 60;
const MAX_BODY_LEN = 120;

self.addEventListener('install', () => {
  // Take over immediately on first install so the very first push after
  // pairing isn't missed while an old SW controls the page.
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('push', (event) => {
  event.waitUntil(handlePush(event));
});

async function handlePush(event) {
  let data = {};
  try {
    if (event.data) data = event.data.json();
  } catch {
    // Non-JSON or empty — still show a generic notification so the user
    // knows something is happening. The PWA will fetch the real state
    // over WS when it opens.
  }

  const title = clamp(typeof data.title === 'string' ? data.title : 'Merlin', MAX_TITLE_LEN) || 'Merlin';
  const body  = clamp(typeof data.body  === 'string' ? data.body  : 'Needs your attention', MAX_BODY_LEN) || 'Needs your attention';
  const type  = typeof data.t === 'string' ? data.t : '';
  const id    = typeof data.id === 'string' ? data.id.slice(0, 64) : '';

  await self.registration.showNotification(title, {
    body,
    tag: NOTIFICATION_TAG,           // Coalesce — don't stack 5 notifications for 5 approvals.
    renotify: true,                  // But still buzz if the user hasn't opened the last one.
    requireInteraction: false,       // Auto-dismiss; tap-to-open is the happy path.
    icon: '/icon-192.png',           // Falls back to favicon if the file is missing.
    badge: '/badge-72.png',
    data: { t: type, id, ts: Date.now() },
  });
}

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  event.waitUntil(focusOrOpen());
});

async function focusOrOpen() {
  const url = new URL('/', self.location.origin).href;
  const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
  for (const w of windows) {
    // Same-origin only — ignore arbitrary foreign tabs.
    if (new URL(w.url).origin === self.location.origin) {
      try { await w.focus(); return; } catch {}
    }
  }
  if (self.clients.openWindow) {
    await self.clients.openWindow(url);
  }
}

function clamp(s, n) {
  if (typeof s !== 'string') return '';
  return s.length > n ? s.slice(0, n) : s;
}
