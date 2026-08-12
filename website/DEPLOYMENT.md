# Website Deployment

The Astro 7 site is fully prerendered into `dist/` and deployed as Cloudflare
static assets. There is no request-time Worker bundle, session binding, or
runtime GitHub API call.

## Cloudflare Git Integration

Connect the repository to a Cloudflare Workers project with these settings:

- Project name: `bub`
- Working directory: `website`
- Build command: `pnpm install --frozen-lockfile && pnpm check && pnpm build && pnpm verify:build`
- Deploy command: `pnpm exec wrangler deploy --config wrangler.jsonc`
- Node version: `24.19.0`
- pnpm version: `11.21.0`
- Environment variable: `SITE_URL=https://bub.build`
- Build secret: `GITHUB_TOKEN=<GitHub PAT>` (optional; raises GitHub API limits)

The prebuild script snapshots repository stars and contributors into generated
data. Deployment never calls GitHub at request time.

[`wrangler.jsonc`](./wrangler.jsonc) is the deployment source of truth. It:

- serves `dist/` through Cloudflare Static Assets;
- preserves Astro's trailing-slash routes;
- serves `404.html` with a real HTTP 404 status;
- pins the newest compatibility date supported by the locked workerd runtime;
- disables Worker observability because no Worker code is deployed.

A successful dry-run reports the static asset scan, a tiny generated assets
router, and `No bindings found`.

## Local Verification

Run the same gates used by CI:

```bash
pnpm install --frozen-lockfile
pnpm peers check
pnpm audit --audit-level=low
pnpm check
pnpm build
pnpm verify:build
pnpm verify:deploy
pnpm test:preview
```

`verify:build` derives expected routes from the content collections and checks
HTML routes, Pagefind fragments, sitemap URLs, RSS feeds, Blog metadata,
pagination, edit links, and OG image dimensions. `test:preview` starts the
locked Wrangler/workerd runtime and checks representative English, Chinese,
docs, Blog, feed, image, sitemap, and 404 responses.

Production deployment remains owned by Cloudflare Git integration. GitHub
Actions performs the full read-only validation sequence on every pull request
and push to `main`; release workflows continue to handle Python package release
tasks only.
