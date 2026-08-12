import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const websiteDir = fileURLToPath(new URL('..', import.meta.url));
const wrangler = path.join(
  websiteDir,
  'node_modules',
  '.bin',
  process.platform === 'win32' ? 'wrangler.cmd' : 'wrangler',
);
const result = spawnSync(
  wrangler,
  ['deploy', '--config', 'wrangler.jsonc', '--dry-run'],
  { cwd: websiteDir, encoding: 'utf8', env: { ...process.env, NO_COLOR: '1' } },
);
const output = `${result.stdout ?? ''}${result.stderr ?? ''}`;

process.stdout.write(output);
assert.equal(result.error, undefined, 'Wrangler could not be started.');
assert.equal(result.status, 0, `Wrangler dry-run failed with status ${result.status}.`);
assert.match(output, /No bindings found\./, 'Cloudflare deployment must not declare runtime bindings.');

console.log('Verified an assets-only Cloudflare deployment with no runtime bindings.');
