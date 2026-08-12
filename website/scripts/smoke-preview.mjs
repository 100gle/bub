import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const websiteDir = fileURLToPath(new URL('..', import.meta.url));
const port = Number.parseInt(process.env.BUB_PREVIEW_PORT ?? '8792', 10);
const origin = `http://127.0.0.1:${port}`;
const wrangler = path.join(
  websiteDir,
  'node_modules',
  '.bin',
  process.platform === 'win32' ? 'wrangler.cmd' : 'wrangler',
);
const output = [];

const server = spawn(
  wrangler,
  ['dev', '--config', 'wrangler.jsonc', '--ip', '127.0.0.1', '--port', String(port)],
  {
    cwd: websiteDir,
    env: { ...process.env, NO_COLOR: '1' },
    stdio: ['ignore', 'pipe', 'pipe'],
  },
);

for (const stream of [server.stdout, server.stderr]) {
  stream.setEncoding('utf8');
  stream.on('data', (chunk) => {
    output.push(chunk);
    if (output.length > 200) output.shift();
  });
}

async function stopServer() {
  if (server.exitCode !== null || server.signalCode !== null) return;
  server.kill('SIGTERM');
  await Promise.race([
    new Promise((resolve) => server.once('exit', resolve)),
    new Promise((resolve) => setTimeout(resolve, 5_000)),
  ]);
  if (server.exitCode === null && server.signalCode === null) server.kill('SIGKILL');
}

async function waitUntilReady() {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (server.exitCode !== null) {
      throw new Error(`Wrangler exited with code ${server.exitCode}.\n${output.join('')}`);
    }
    try {
      const response = await fetch(origin);
      if (response.ok) return;
    } catch {
      // The socket is expected to reject connections until workerd is ready.
    }
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
  throw new Error(`Wrangler did not become ready within 30 seconds.\n${output.join('')}`);
}

const routes = [
  { path: '/', status: 200, type: 'text/html', contains: '<html lang="en"' },
  { path: '/zh-cn/', status: 200, type: 'text/html', contains: '<html lang="zh-CN"' },
  { path: '/docs/getting-started/', status: 200, type: 'text/html', contains: 'data-pagefind-body' },
  {
    path: '/zh-cn/docs/getting-started/',
    status: 200,
    type: 'text/html',
    contains: '<html lang="zh-CN"',
  },
  { path: '/posts/', status: 200, type: 'text/html', contains: 'application/rss+xml' },
  { path: '/zh-cn/posts/', status: 200, type: 'text/html', contains: '<html lang="zh-CN"' },
  {
    path: '/posts/socialized-evaluation/',
    status: 200,
    type: 'text/html',
    contains: '"@type":"BlogPosting"',
  },
  {
    path: '/zh-cn/posts/socialized-evaluation/',
    status: 200,
    type: 'text/html',
    contains: '"inLanguage":"zh-CN"',
  },
  { path: '/rss.xml', status: 200, type: 'application/xml', contains: '<rss version="2.0">' },
  {
    path: '/zh-cn/rss.xml',
    status: 200,
    type: 'application/xml',
    contains: '<language>zh-CN</language>',
  },
  { path: '/og/index.png', status: 200, type: 'image/png' },
  { path: '/og/zh-cn/index.png', status: 200, type: 'image/png' },
  { path: '/sitemap-index.xml', status: 200, type: 'application/xml', contains: '<sitemapindex' },
  { path: '/missing', status: 404, type: 'text/html', contains: 'Page not found' },
  {
    path: '/zh-cn/missing',
    status: 404,
    type: 'text/html',
    contains: '<html lang="zh-CN"',
  },
];

try {
  await waitUntilReady();
  for (const route of routes) {
    const response = await fetch(`${origin}${route.path}`, { redirect: 'manual' });
    assert.equal(response.status, route.status, `${route.path} status`);
    assert.match(response.headers.get('content-type') ?? '', new RegExp(route.type), `${route.path} content type`);
    if (route.contains) {
      assert.ok((await response.text()).includes(route.contains), `${route.path} response content`);
    }
  }
  console.log(`Verified ${routes.length} routes through Wrangler's local Cloudflare runtime.`);
} catch (error) {
  console.error(output.join(''));
  throw error;
} finally {
  await stopServer();
}
