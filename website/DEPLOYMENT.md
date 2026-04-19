# Website Deployment

## Goal

The new `website/` directory is the docs and marketing site for Bub.

Legacy MkDocs source files may still exist in the repository during the
transition, but production deployment now targets the Astro site on
Cloudflare Pages.

## Cloudflare Pages

Connect the repository to a Cloudflare Pages project with Git integration.

Recommended settings:

- Production branch: `main`
- Root directory: `website`
- Build command: `pnpm install --frozen-lockfile && pnpm build`
- Build output directory: `dist`
- Environment variable: `SITE_URL=https://bub.build`
- Environment variable: `NODE_VERSION=22.16.0`

The repo also includes [wrangler.jsonc](./wrangler.jsonc) so local preview and
Cloudflare Pages runtime settings stay aligned:

- `pages_build_output_dir = "./dist"`
- `compatibility_flags = ["nodejs_compat"]`

Production deployment is driven by GitHub release automation instead of
push-based auto deploys.

## Current Repo State

The local developer entrypoints now target the new site:

- `just docs`
- `just docs-test`
- `just docs-preview`

The CI docs check also builds `website/` instead of MkDocs.

## GitHub Actions Deployment

The deployment split is intentionally simple:

- `main.yml` only verifies that the website builds
- `on-release-main.yml` deploys production when a GitHub release is published

Required repository configuration:

- GitHub Actions secret: `CLOUDFLARE_API_TOKEN`
- GitHub Actions secret: `CLOUDFLARE_ACCOUNT_ID`
- GitHub Actions variable: `CLOUDFLARE_PAGES_PROJECT_NAME`

## Cutover Later

Once the Cloudflare Pages project is live and verified, the remaining cleanup
work is:

1. remove legacy MkDocs source files once they are no longer needed
2. update repository docs that still describe the old docs toolchain
