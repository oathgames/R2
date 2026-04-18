#!/usr/bin/env node
// Merlin — brand-scraper CLI harness
//
// Standalone entry point for exercising the scraper against any live URL
// without running the full Merlin app. Launches a hidden Electron process,
// scrapes the URL, prints BrandSignal JSON to stdout, exits.
//
// Usage:
//   npx electron app/brand-scraper-cli.js <url>
//   npx electron app/brand-scraper-cli.js <url> --out=signal.json
//
// Exit codes:
//   0  success (JSON written to stdout or --out file)
//   1  invalid args
//   2  scrape failed (network, timeout, etc.)

'use strict';

const { app } = require('electron');
const fs = require('fs');
const path = require('path');
const { scrapeBrand } = require('./brand-scraper');

function parseArgs(argv) {
  // Electron prepends its own arg list; find the first non-flag, non-electron arg.
  const args = argv.slice(1).filter(a => !a.startsWith('--enable-') && !a.endsWith('brand-scraper-cli.js'));
  let url = null;
  let outPath = null;
  let includeScreenshots = false;
  for (const a of args) {
    if (a.startsWith('--out=')) outPath = a.slice(6);
    else if (a === '--screenshots') includeScreenshots = true;
    else if (!a.startsWith('--') && !url) url = a;
  }
  return { url, outPath, includeScreenshots };
}

async function run() {
  const { url, outPath, includeScreenshots } = parseArgs(process.argv);
  if (!url) {
    process.stderr.write('usage: electron brand-scraper-cli.js <url> [--out=file.json] [--screenshots]\n');
    app.exit(1);
    return;
  }

  try {
    await app.whenReady();
    const result = await scrapeBrand(url);

    // Screenshots are large (~1-3MB base64 each). Default to stripping them
    // from the CLI output so the JSON is human-scannable. Pass --screenshots
    // to keep them for a full round-trip test.
    if (!includeScreenshots && result.screenshots) {
      result.screenshots = {
        desktop: '[base64 png elided — pass --screenshots to include]',
        mobile: '[base64 png elided — pass --screenshots to include]',
      };
    }

    const json = JSON.stringify(result, null, 2);
    if (outPath) {
      fs.writeFileSync(path.resolve(outPath), json);
      process.stderr.write(`wrote ${json.length} bytes to ${outPath}\n`);
    } else {
      process.stdout.write(json + '\n');
    }
    app.exit(0);
  } catch (err) {
    process.stderr.write(`scrape failed: ${err && err.stack || err}\n`);
    app.exit(2);
  }
}

run();
