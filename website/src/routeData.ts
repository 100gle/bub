import { defineRouteMiddleware } from '@astrojs/starlight/route-data';

const DEFAULT_SITE_URL = 'https://bub.build';

function normalizeRoute(pathname: string): string {
  return pathname.replace(/^\/+|\/+$/g, '') || 'index';
}

function upsertMetaTag(
  head: Array<{ tag: string; attrs?: Record<string, unknown> }>,
  key: 'property' | 'name',
  value: string,
  content: string,
) {
  const existingTag = head.find((entry) => entry.tag === 'meta' && entry.attrs?.[key] === value);

  if (existingTag) {
    existingTag.attrs = { ...existingTag.attrs, content };
    return;
  }

  head.push({
    tag: 'meta',
    attrs: { [key]: value, content },
  });
}

export const onRequest = defineRouteMiddleware((context) => {
  const route = normalizeRoute(context.url.pathname);
  const site = context.site ?? new URL(DEFAULT_SITE_URL);
  const imageUrl = new URL(`/og/${route}.png`, site).toString();
  const { head } = context.locals.starlightRoute;

  upsertMetaTag(head, 'property', 'og:image', imageUrl);
  upsertMetaTag(head, 'property', 'og:image:width', '1200');
  upsertMetaTag(head, 'property', 'og:image:height', '630');
  upsertMetaTag(head, 'name', 'twitter:image', imageUrl);
});
