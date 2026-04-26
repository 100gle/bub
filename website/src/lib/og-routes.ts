function trimRoute(route: string): string {
  return route.replace(/^\/+|\/+$/g, '');
}

export function normalizeOgRouteFromPathname(pathname: string): string {
  return trimRoute(pathname) || 'index';
}

export function normalizeOgRouteFromEntryId(id: string): string {
  const route = id.replace(/\.(md|mdx)$/, '').replace(/(?:^|\/)index$/, '');
  return normalizeOgRouteFromPathname(route);
}

export function createOgImageUrl(route: string, site: URL): string {
  return new URL(`/og/${route}.png`, site).toString();
}
