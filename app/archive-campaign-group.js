// Pure helper that groups live ads (from ads-live.json) by campaignName so
// the Archive "Ads" tab can render campaign-scoped sections instead of a flat
// firehose. Keeping this out of renderer.js makes it unit-testable from Node
// without booting Electron.
//
// Input:  ads — array in the exact shape returned by `merlin.getLiveAds()`.
//         Each entry has at minimum `{ adId, platform }`; `campaignName`,
//         `campaignId`, `spend`, `status` may or may not be present.
//
// Output: { groups, flatAds, showHeaders } where:
//   groups      — [{ key, name, platform, ads, totalSpend }]
//                 sorted by totalSpend desc, with the Uncategorized bucket
//                 (ads without a campaignName) pinned last.
//   flatAds     — the input ads re-ordered so all members of a group are
//                 adjacent, preserving intra-group order.
//   showHeaders — true iff there are 2+ buckets. Single-bucket brands don't
//                 need a header (prevents a lone "Uncategorized" label from
//                 appearing on legacy or non-Meta-only brands where
//                 campaign fields aren't populated yet).
//
// Grouping key: `${platform}::${campaignName}`. Platform is part of the key
// so a coincidentally-named campaign on two different ad platforms stays
// separated — users launching both "Meta — Scaling" and "TikTok — Scaling"
// should see two cards with distinct platform badges, not one merged group.
//
// The "no campaign" bucket intentionally uses a sentinel string rather than
// null / undefined so callers can safely use it as a Map key or switch
// target without tripping on falsy-key bugs.
//
// Dual-module shim: loaded as a global (window.MerlinArchiveCampaignGroup)
// in the renderer, and as a CommonJS module in the node-based test harness.
// Keep both export paths intact when editing.

(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    module.exports = factory();
  } else {
    root.MerlinArchiveCampaignGroup = factory();
  }
}(typeof self !== 'undefined' ? self : this, function () {
  const NO_CAMPAIGN_KEY = '__no_campaign__';

  // Single source of truth for the campaign-grouping key. renderer.js calls
  // this to match each ad card back to its group header; groupLiveAdsByCampaign
  // calls this when bucketing. Both paths MUST agree or ad cards will render
  // under the wrong (or no) header. If you tweak the rule (e.g. add a
  // normalization step), every caller updates in one place — and the
  // "stability vs. grouper" test in archive-campaign-group.test.js will fail
  // loudly if a future change forgets to route through here.
  function makeAdCampaignKey(ad) {
    if (!ad || typeof ad !== 'object') return NO_CAMPAIGN_KEY;
    const name = typeof ad.campaignName === 'string' ? ad.campaignName.trim() : '';
    if (!name) return NO_CAMPAIGN_KEY;
    const platform = typeof ad.platform === 'string' ? ad.platform : '';
    return `${platform.toLowerCase()}::${name}`;
  }

  function groupLiveAdsByCampaign(ads) {
    if (!Array.isArray(ads)) {
      return { groups: [], flatAds: [], showHeaders: false };
    }

    const groupMap = new Map();
    for (const ad of ads) {
      if (!ad || typeof ad !== 'object') continue;
      const key = makeAdCampaignKey(ad);
      // The group's display `name` is the trimmed campaign name for named
      // buckets, or '' for the Uncategorized bucket (renderer substitutes
      // 'Uncategorized' at render time). Recomputing here keeps the key +
      // display values derived from the same inputs in the same function.
      const name = key === NO_CAMPAIGN_KEY
        ? ''
        : (typeof ad.campaignName === 'string' ? ad.campaignName.trim() : '');
      const platform = typeof ad.platform === 'string' ? ad.platform : '';
      let group = groupMap.get(key);
      if (!group) {
        group = { key, name, platform, ads: [], totalSpend: 0 };
        groupMap.set(key, group);
      }
      group.ads.push(ad);
      const spend = Number(ad.spend);
      if (Number.isFinite(spend) && spend > 0) {
        group.totalSpend += spend;
      }
    }

    const groups = Array.from(groupMap.values()).sort((a, b) => {
      // Pin the Uncategorized bucket to the end. Within named groups, order
      // by total spend desc (the decision-weight signal), falling back to
      // ad-count desc and finally name for deterministic ordering.
      if (a.key === NO_CAMPAIGN_KEY) return 1;
      if (b.key === NO_CAMPAIGN_KEY) return -1;
      if (b.totalSpend !== a.totalSpend) return b.totalSpend - a.totalSpend;
      if (b.ads.length !== a.ads.length) return b.ads.length - a.ads.length;
      return a.name.localeCompare(b.name);
    });

    const flatAds = [];
    for (const g of groups) {
      for (const ad of g.ads) flatAds.push(ad);
    }

    const showHeaders = groups.length > 1;
    return { groups, flatAds, showHeaders };
  }

  return { groupLiveAdsByCampaign, makeAdCampaignKey, NO_CAMPAIGN_KEY };
}));
