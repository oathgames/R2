#!/usr/bin/env node
// test/run-routing-corpus.js
//
// Static routing-corpus checks. Does NOT invoke the LLM — that lives in the
// e2e harness (not shipped to users). This runner guards against:
//
//   1. Corpus JSON malformed or out of sync.
//   2. `expectedSkill` pointing at a skill that no longer exists.
//   3. A skill with zero corpus coverage — surface that gap in CI output.
//
// If any new skill lands without a single corpus entry, PR is blocked until
// at least one canonical utterance is added.

'use strict';

const fs = require('fs');
const path = require('path');
const { listSkills } = require('./validate-skills');

const CORPUS_PATH = path.join(__dirname, 'routing-corpus.json');

function main() {
  if (!fs.existsSync(CORPUS_PATH)) {
    console.error(`✗ ${CORPUS_PATH} is missing`);
    process.exit(1);
  }
  let corpus;
  try {
    corpus = JSON.parse(fs.readFileSync(CORPUS_PATH, 'utf8'));
  } catch (err) {
    console.error(`✗ routing-corpus.json is not valid JSON: ${err.message}`);
    process.exit(1);
  }
  if (!Array.isArray(corpus.entries) || corpus.entries.length === 0) {
    console.error('✗ routing-corpus.json `entries` must be a non-empty array');
    process.exit(1);
  }

  const skills = new Set(listSkills().map((s) => s.name));
  const coverage = new Map();
  for (const s of skills) coverage.set(s, 0);

  const errors = [];
  for (const [i, entry] of corpus.entries.entries()) {
    if (!entry.utterance || typeof entry.utterance !== 'string') {
      errors.push(`entry[${i}]: missing/invalid \`utterance\``);
      continue;
    }
    if (!entry.expectedSkill || typeof entry.expectedSkill !== 'string') {
      errors.push(`entry[${i}]: missing/invalid \`expectedSkill\``);
      continue;
    }
    if (!skills.has(entry.expectedSkill)) {
      errors.push(
        `entry[${i}] ("${entry.utterance}"): expectedSkill "${entry.expectedSkill}" does not exist — corpus is stale`
      );
      continue;
    }
    coverage.set(entry.expectedSkill, (coverage.get(entry.expectedSkill) || 0) + 1);
  }

  const uncovered = [...coverage.entries()]
    .filter(([, n]) => n === 0)
    .map(([name]) => name);
  if (uncovered.length > 0) {
    errors.push(
      `Skills with zero corpus entries: ${uncovered.join(', ')} — add canonical utterances before merging.`
    );
  }

  if (errors.length > 0) {
    console.error(`\n${errors.length} error(s):`);
    for (const e of errors) console.error(`  ✗ ${e}`);
    process.exit(1);
  }
  console.log(
    `✓ ${corpus.entries.length} utterances covering ${skills.size} skills (min coverage: ${Math.min(...coverage.values())})`
  );
}

if (require.main === module) main();
