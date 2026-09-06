// Synchronise arithmetic documentation only, without serialising runtime objects.
// The full bootstrap generator predates extracted generic collection packages.
// Refuse unexpected source/bootstrap shapes rather than changing executable data.
import { readFileSync, writeFileSync } from 'node:fs';

const sourcePath = 'core/data/core.joke';
const outputPath = 'core/a_generated_bootstrap_payloads.go';
const source = readFileSync(sourcePath, 'utf8');
let output = readFileSync(outputPath, 'utf8');
const names = ['inc', '+', '*', '-', 'dec'];
for (const name of names) {
  const marker = `(defn ${name}\n  "`;
  const start = source.indexOf(marker);
  if (start < 0 || source.indexOf(marker, start + marker.length) >= 0) {
    throw new Error(`Expected one source definition for ${name}`);
  }
  const docStart = start + marker.length;
  const end = source.indexOf('"', docStart);
  const doc = source.slice(docStart, end);
  if (end < 0 || doc.includes('\\')) throw new Error(`Unsupported docstring for ${name}`);
  const legacy = doc.replace('Promotes integer overflow\n  to arbitrary-precision integers.', 'Does not auto-promote\n  ints, will overflow.');
  const desired = JSON.stringify(doc);
  const old = JSON.stringify(legacy);
  const count = (text: string) => output.split(text).length - 1;
  if (count(desired) === 1) continue;
  if (legacy === doc || count(old) !== 1) throw new Error(`Unexpected bootstrap documentation for ${name}`);
  output = output.replace(old, desired);
}
if (process.argv.includes('--check')) {
  if (output !== readFileSync(outputPath, 'utf8')) throw new Error('Arithmetic bootstrap docs are stale');
} else {
  writeFileSync(outputPath, output);
}
