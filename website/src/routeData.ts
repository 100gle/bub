import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { createOgImageUrl, normalizeOgRouteFromPathname } from '@/lib/og-routes';

interface OgImageMeta {
  url: string;
  width?: string;
  height?: string;
}

interface CoverImage {
  src: string;
  width?: number | string;
  height?: number | string;
}

interface RouteDataContext {
  locals: { starlightRoute: { entry: { data: { cover?: CoverImage } } } };
  site: URL;
  url: URL;
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

function normalizeDimension(value: unknown): string | undefined {
  return typeof value === 'number' || typeof value === 'string' ? String(value) : undefined;
}

function getDocOgImage(context: RouteDataContext): OgImageMeta {
  const cover = context.locals.starlightRoute.entry.data.cover;

  if (cover) {
    return {
      url: new URL(cover.src, context.site).toString(),
      width: normalizeDimension(cover.width),
      height: normalizeDimension(cover.height),
    };
  }

  const route = normalizeOgRouteFromPathname(context.url.pathname);
  return {
    url: createOgImageUrl(route, context.site),
    width: '1200',
    height: '630',
  };
}

export const onRequest = defineRouteMiddleware((context) => {
  if (!context.site) {
    throw new Error('Astro site URL must be configured for docs OG image metadata.');
  }

  const image = getDocOgImage(context);
  const { head } = context.locals.starlightRoute;

  upsertMetaTag(head, 'property', 'og:image', image.url);
  if (image.width) upsertMetaTag(head, 'property', 'og:image:width', image.width);
  if (image.height) upsertMetaTag(head, 'property', 'og:image:height', image.height);
  upsertMetaTag(head, 'name', 'twitter:image', image.url);
});
