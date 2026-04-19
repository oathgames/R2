// Browser-only preview harness: stubs the Electron contextBridge so renderer.js
// can run to completion under `npx serve`. Not shipped; gated to preview-harness.html.
//
// Strategy: Proxy all missing methods to a universal no-op so renderer.js can call
// any `merlin.*` or `merlin.onX(...)` without throwing, even ones we haven't listed.
(function installMerlinStub() {
  const noop = () => {};
  const asyncUndef = () => Promise.resolve();

  // Explicit overrides for calls whose return shape matters for renderer boot
  const overrides = {
    platform: 'win32',
    __previewHarness: true,

    // Subscription / licensing
    getSubscription: async () => ({ status: 'active', tier: 'pro', trialDaysLeft: 7 }),
    activateKey: async () => ({ success: true }),
    triggerClaudeLogin: async () => ({ success: false }),

    // Version + updates
    getVersion: async () => ({ version: '1.0.5-preview' }),
    checkForUpdates: async () => ({ hasUpdate: false }),

    // Brands / config
    getBrands: async () => [
      { name: 'acme', vertical: 'saas', productName: 'Acme SaaS' },
      { name: 'riverbed', vertical: 'ecommerce', productName: 'Riverbed Tees' },
      { name: 'pixel-forge', vertical: 'games', productName: 'Pixel Forge' },
    ],
    getBrand: async (name) => ({ name, vertical: 'saas' }),
    saveConfigField: async (field, value, brand) => {
      console.log('[preview-stub] saveConfigField', { field, value, brand });
      return { ok: true };
    },

    // State + briefing + setup
    loadState: async () => ({ activeBrand: 'acme' }),
    saveState: async () => ({}),
    getBriefing: async () => null,
    dismissBriefing: noop,
    checkSetup: async () => ({ needsSetup: false, completed: true }),
    startSession: noop,

    // Mobile QR pairing — returns a placeholder QR-shape so the modal has
    // something to render without round-tripping through the real relay.
    getMobileQR: async () => ({
      qrDataUri: 'data:image/svg+xml;utf8,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="220" height="220" viewBox="0 0 20 20"><rect width="20" height="20" fill="#fff"/><g fill="#000"><rect x="1" y="1" width="6" height="6"/><rect x="13" y="1" width="6" height="6"/><rect x="1" y="13" width="6" height="6"/><rect x="9" y="9" width="2" height="2"/><rect x="11" y="11" width="2" height="2"/><rect x="15" y="11" width="2" height="2"/></g></svg>'),
      pwaUrl: 'https://relay.merlingotme.com/p/preview-abc123',
    }),

    // Connection status — light up many tiles so filtering is visible
    getConnectionStatus: async () => ({
      meta: true, tiktok: true, google: true, amazon: true, reddit: true,
      shopify: true, stripe: true, klaviyo: true, slack: true, discord: true,
      linkedin: false, etsy: true, pinterest: false, snapchat: false,
    }),
  };

  // Universal handler: any `onX(cb)` registration returns an unsubscribe fn.
  // Any other unknown method resolves to undefined (async) or no-op (sync) —
  // the shape rarely matters for boot.
  const handler = {
    get(target, prop) {
      if (prop in target) return target[prop];
      const name = String(prop);
      if (name.startsWith('on') && name.length > 2 && name[2] >= 'A' && name[2] <= 'Z') {
        return () => noop; // event registration → unsubscribe
      }
      return asyncUndef;
    },
  };

  window.merlin = new Proxy(overrides, handler);
})();
