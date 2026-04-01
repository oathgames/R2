AutoCMO — AI Content Engine
============================

1. Open this folder in Claude Code
2. Type: /cmo

First run walks you through API key setup (only fal.ai key needed)
and auto-builds your brand profile from your website.

Then just talk naturally:

  /cmo cream-set product video
  /cmo pink-set make 3 image variations
  /cmo clone my voice
  /cmo check Meta performance
  /cmo write a blog post about the bonefish hoodie

Adding products:
  Create a folder under assets/brands/<brand>/products/ with a references/ subfolder.
  Drop product photos in it. Claude handles the rest.

  Example:
    assets/brands/madchill/products/new-jacket/references/photo1.jpg

  Next time you run /cmo new-jacket, it just works.

Quality benchmarks:
  Drop 3-5 high-quality ad examples into assets/brands/<brand>/quality-benchmark/.
  Claude scores every generated image/video against these. Keeps the bar high.

Voice cloning:
  Drop 1-3 audio recordings (.mp3/.wav) into assets/brands/<brand>/voices/.
  Then: /cmo clone my voice — creates a custom ElevenLabs voice for voiceovers.

Avatars:
  Drop a photo (.jpg/.png) or video (.mp4) into assets/brands/<brand>/avatars/.
  Photos use HeyGen talking-head mode. Videos use Kling lip-sync.

Batch mode:
  /cmo make 3 variations of cream-set

SEO Blog:
  /cmo write a blog post for northswell
  Auto-generates a 600-1000 word SEO article and publishes to Shopify.
  Runs automatically each daily cycle if Shopify is configured.

Meta Ads:
  /cmo push to Meta          → uploads to Testing campaign ($5/day)
  /cmo check Meta performance → shows CTR, CPC, ROAS, flags winners/losers

TikTok Ads:
  /cmo push to TikTok          → uploads to TikTok Testing campaign ($5/day)
  /cmo check TikTok performance → shows CTR, CPC, ATC, flags winners/losers

Maintenance:
  /cmo archive old results   → moves results older than 30 days to archive/
