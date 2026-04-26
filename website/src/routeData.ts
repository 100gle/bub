import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { createOgImageUrl, normalizeOgRouteFromPathname } from '@/lib/og-routes';

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
  if (!context.site) {
    throw new Error('Astro site URL must be configured for docs OG image metadata.');
  }

  const route = normalizeOgRouteFromPathname(context.url.pathname);
  const imageUrl = createOgImageUrl(route, context.site);
  const { head } = context.locals.starlightRoute;

  upsertMetaTag(head, 'property', 'og:image', imageUrl);
  upsertMetaTag(head, 'property', 'og:image:width', '1200');
  upsertMetaTag(head, 'property', 'og:image:height', '630');
  upsertMetaTag(head, 'name', 'twitter:image', imageUrl);
});
