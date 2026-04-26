const INDEX_ROUTE_PATTERN = /(?:^|\/)index$/;

function trimRoute(route: string): string {
  return route.replace(/^\/+|\/+$/g, '');
}

export function normalizeOgRouteFromPathname(pathname: string): string {
  return trimRoute(pathname) || 'index';
}

export function normalizeOgRouteFromEntryId(id: string): string {
  // Strip the source file extension and collapse trailing `/index` docs files
  // onto their directory route so content entry IDs match page URLs.
  const route = id.replace(/\.(md|mdx)$/, '').replace(INDEX_ROUTE_PATTERN, '');
  return normalizeOgRouteFromPathname(route);
}

export function createOgImageUrl(route: string, site: URL): string {
  return new URL(`/og/${route}.png`, site).toString();
}
