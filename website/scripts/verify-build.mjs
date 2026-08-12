import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { gunzipSync } from 'node:zlib';
import sharp from 'sharp';

const websiteDir = fileURLToPath(new URL('..', import.meta.url));
const distDir = path.join(websiteDir, 'dist');
const docsDir = path.join(websiteDir, 'src/content/docs');
const postsDir = path.join(websiteDir, 'src/content/posts');
const siteOrigin = 'https://bub.build';

async function walk(directory, extensions) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map(async (entry) => {
      const entryPath = path.join(directory, entry.name);
      if (entry.isDirectory()) return walk(entryPath, extensions);
      return extensions.has(path.extname(entry.name)) ? [entryPath] : [];
    }),
  );
  return files.flat();
}

function routeFileFromContent(file, contentDir) {
  const entryId = path.relative(contentDir, file).replaceAll(path.sep, '/').replace(/\.(md|mdx)$/, '');
  return entryId.endsWith('/index') ? `${entryId}.html` : `${entryId}/index.html`;
}

function routeFileFromPost(file) {
  const [locale, filename] = path.relative(postsDir, file).split(path.sep);
  const prefix = locale === 'en' ? '' : `${locale}/`;
  const slug = filename.replace(/\.(md|mdx)$/, '');
  return `${prefix}posts/${slug}/index.html`;
}

function routeUrlFromHtml(file) {
  const pathname = file === 'index.html' ? '/' : `/${file.replace(/index\.html$/, '')}`;
  return new URL(pathname, siteOrigin).href;
}

function occurrences(value, pattern) {
  return value.match(pattern)?.length ?? 0;
}

function unescapeXml(value) {
  return value
    .replaceAll('&amp;', '&')
    .replaceAll('&lt;', '<')
    .replaceAll('&gt;', '>')
    .replaceAll('&quot;', '"')
    .replaceAll('&apos;', "'");
}

function tagValues(xml, tag) {
  return [...xml.matchAll(new RegExp(`<${tag}>(?:<!\\[CDATA\\[)?([\\s\\S]*?)(?:\\]\\]>)?</${tag}>`, 'g'))]
    .map((match) => unescapeXml(match[1].trim()));
}

async function readDist(relativePath) {
  return readFile(path.join(distDir, relativePath), 'utf8');
}

function assertContains(value, needle, message) {
  assert.ok(value.includes(needle), message);
}

assert.ok(existsSync(distDir), 'dist/ is missing; run pnpm build first.');

const [docSources, postSources, builtHtml, pagefindFragments] = await Promise.all([
  walk(docsDir, new Set(['.md', '.mdx'])),
  walk(postsDir, new Set(['.md', '.mdx'])),
  walk(distDir, new Set(['.html'])),
  walk(path.join(distDir, 'pagefind/fragment'), new Set(['.pf_fragment'])),
]);

const expectedHtml = new Set([
  '404.html',
  'index.html',
  'zh-cn/index.html',
  'zh-cn/404.html',
  'posts/index.html',
  'zh-cn/posts/index.html',
  ...docSources.map((file) => routeFileFromContent(file, docsDir)),
  ...postSources.map(routeFileFromPost),
]);
const actualHtml = new Set(
  builtHtml.map((file) => path.relative(distDir, file).replaceAll(path.sep, '/')),
);

assert.deepEqual(actualHtml, expectedHtml, 'The built HTML route set does not match the content sources.');
assert.equal(
  pagefindFragments.length,
  docSources.length + postSources.length,
  'Pagefind must index every docs page and Blog article, but not list or landing pages.',
);

const indexedUrls = new Set(
  await Promise.all(pagefindFragments.map(async (fragment) => {
    const compressed = await readFile(fragment);
    const decoded = gunzipSync(compressed).toString('utf8');
    assert.ok(decoded.startsWith('pagefind_dcd'), `${fragment} has an unexpected Pagefind payload.`);
    return JSON.parse(decoded.slice('pagefind_dcd'.length)).url;
  })),
);
const expectedIndexedUrls = new Set([
  ...docSources
    .map((file) => routeUrlFromHtml(routeFileFromContent(file, docsDir)))
    .map((url) => new URL(url).pathname),
  ...postSources
    .map((file) => routeUrlFromHtml(routeFileFromPost(file)))
    .map((url) => new URL(url).pathname),
]);
assert.deepEqual(
  indexedUrls,
  expectedIndexedUrls,
  'Pagefind must contain each docs and Blog article URL exactly once.',
);

const postsByLocale = Map.groupBy(postSources, (file) =>
  path.relative(postsDir, file).split(path.sep)[0],
);

for (const [locale, files] of postsByLocale) {
  assert.ok(locale === 'en' || locale === 'zh-cn', `Unexpected post locale directory: ${locale}`);
  const prefix = locale === 'en' ? '' : '/zh-cn';
  const rssPath = locale === 'en' ? 'rss.xml' : 'zh-cn/rss.xml';
  const rssHref = `${prefix}/rss.xml`;
  const listPath = locale === 'en' ? 'posts/index.html' : 'zh-cn/posts/index.html';
  const listHtml = await readDist(listPath);

  assert.equal(occurrences(listHtml, /type="application\/rss\+xml"/g), 1, `${listPath} RSS discovery`);
  assertContains(listHtml, `href="${rssHref}"`, `${listPath} must expose a visible RSS link.`);
  assert.equal(occurrences(listHtml, /data-pagefind-body/g), 0, `${listPath} must not duplicate articles in search.`);
  assert.ok(
    listHtml.lastIndexOf('class="starlight-site-footer') > listHtml.lastIndexOf('</main>'),
    `${listPath} site footer must be a top-level landmark outside main.`,
  );
  assertContains(
    listHtml,
    `property="og:locale" content="${locale === 'en' ? 'en_US' : 'zh_CN'}"`,
    `${listPath} Open Graph locale`,
  );
  assertContains(
    listHtml,
    `property="og:image" content="${siteOrigin}${locale === 'en' ? '/og/index.png' : '/og/zh-cn/index.png'}"`,
    `${listPath} localized Open Graph image`,
  );

  const postRoutes = [];
  for (const file of files) {
    const source = await readFile(file, 'utf8');
    const published = source.match(/^date:\s*(.+)$/m)?.[1]?.trim();
    assert.ok(published, `${file} must declare a date.`);
    const slug = path.basename(file).replace(/\.(md|mdx)$/, '');
    postRoutes.push({
      file,
      slug,
      published: new Date(published),
      href: `${prefix}/posts/${slug}/`,
    });
  }
  postRoutes.sort((left, right) => left.published.getTime() - right.published.getTime());

  for (const [index, route] of postRoutes.entries()) {
    const file = route.file;
    const sourceId = path.relative(postsDir, file).replaceAll(path.sep, '/');
    const slug = route.slug;
    const routePath = `${prefix.slice(1)}${prefix ? '/' : ''}posts/${slug}/index.html`;
    const html = await readDist(routePath);
    const canonical = `${siteOrigin}${prefix}/posts/${slug}/`;
    const jsonLdMatch = html.match(/<script type="application\/ld\+json">([^<]+)<\/script>/);

    assert.equal(occurrences(html, /rel="canonical"/g), 1, `${routePath} canonical count`);
    assertContains(html, `href="${canonical}"`, `${routePath} canonical URL`);
    assert.equal(occurrences(html, /type="application\/rss\+xml"/g), 1, `${routePath} RSS discovery`);
    assertContains(html, `href="${siteOrigin}${rssHref}"`, `${routePath} RSS URL`);
    assertContains(html, 'property="og:type" content="article"', `${routePath} Open Graph type`);
    assertContains(
      html,
      `property="og:locale" content="${locale === 'en' ? 'en_US' : 'zh_CN'}"`,
      `${routePath} Open Graph locale`,
    );
    assertContains(html, 'property="article:published_time"', `${routePath} publish date metadata`);
    assertContains(html, 'data-pagefind-body', `${routePath} Pagefind body`);
    assert.ok(
      html.lastIndexOf('class="starlight-site-footer') > html.lastIndexOf('</main>'),
      `${routePath} site footer must be a top-level landmark outside main.`,
    );
    assertContains(
      html,
      `github.com/bubbuild/bub/edit/main/website/src/content/posts/${sourceId}`,
      `${routePath} edit link`,
    );
    assert.ok(jsonLdMatch, `${routePath} must contain BlogPosting JSON-LD.`);

    const jsonLd = JSON.parse(jsonLdMatch[1]);
    assert.equal(jsonLd['@type'], 'BlogPosting', `${routePath} JSON-LD type`);
    assert.equal(jsonLd.url, canonical, `${routePath} JSON-LD canonical URL`);
    assert.equal(jsonLd.inLanguage, locale === 'en' ? 'en' : 'zh-CN', `${routePath} JSON-LD language`);
    assert.deepEqual(
      jsonLd.image,
      [`${siteOrigin}/og/posts/${locale}/${slug}.png`],
      `${routePath} JSON-LD image`,
    );

    const expectedPagination = [postRoutes[index - 1]?.href, postRoutes[index + 1]?.href].filter(Boolean);
    const actualPagination = [...html.matchAll(/<a[^>]+href="([^"]+)"[^>]+rel="(?:prev|next)"/g)]
      .map((match) => match[1]);
    assert.deepEqual(actualPagination.toSorted(), expectedPagination.toSorted(), `${routePath} pagination URLs`);
  }

  const feed = await readDist(rssPath);
  assert.equal(occurrences(feed, /<item>/g), files.length, `${rssPath} item count`);
  assertContains(feed, `<language>${locale === 'en' ? 'en' : 'zh-CN'}</language>`, `${rssPath} language`);
  assertContains(feed, `<link>${siteOrigin}${prefix}/posts/</link>`, `${rssPath} channel URL`);
  assert.deepEqual(
    tagValues(feed, 'item').map((item) => tagValues(item, 'link')[0]),
    postRoutes.toReversed().map((post) => `${siteOrigin}${post.href}`),
    `${rssPath} item URL and publication order`,
  );
}

for (const [relativePath, canonical, language] of [
  ['index.html', `${siteOrigin}/`, 'en'],
  ['zh-cn/index.html', `${siteOrigin}/zh-cn/`, 'zh-CN'],
]) {
  const html = await readDist(relativePath);
  assert.equal(occurrences(html, /rel="canonical"/g), 1, `${relativePath} canonical count`);
  assertContains(html, `href="${canonical}"`, `${relativePath} canonical URL`);
  assertContains(html, `hreflang="${language}"`, `${relativePath} self language alternate`);
  assertContains(html, 'hreflang="x-default"', `${relativePath} x-default alternate`);
  const rssHref = language === 'en' ? `${siteOrigin}/rss.xml` : `${siteOrigin}/zh-cn/rss.xml`;
  assertContains(html, `type="application/rss+xml"`, `${relativePath} RSS discovery`);
  assertContains(html, `href="${rssHref}"`, `${relativePath} localized RSS URL`);
}

const sitemap = await readDist('sitemap-0.xml');
const sitemapUrls = new Set([...sitemap.matchAll(/<loc>([^<]+)<\/loc>/g)].map((match) => match[1]));
const expectedSitemapUrls = new Set(
  [...expectedHtml].filter((file) => !file.endsWith('404.html')).map(routeUrlFromHtml),
);
assert.deepEqual(sitemapUrls, expectedSitemapUrls, 'The sitemap must contain every public HTML route except 404.');

const expectedOgImages = [
  'og/index.png',
  'og/zh-cn/index.png',
  ...postSources.map((file) => {
    const id = path.relative(postsDir, file).replaceAll(path.sep, '/').replace(/\.(md|mdx)$/, '');
    return `og/posts/${id}.png`;
  }),
];
for (const imagePath of expectedOgImages) {
  const metadata = await sharp(path.join(distDir, imagePath)).metadata();
  assert.equal(metadata.width, 1200, `${imagePath} width`);
  assert.equal(metadata.height, 630, `${imagePath} height`);
  assert.equal(metadata.format, 'png', `${imagePath} format`);
}

assert.equal(
  existsSync(path.join(distDir, 'server')),
  false,
  'A fully prerendered site must not emit a server bundle.',
);
assert.equal(existsSync(path.join(distDir, '_worker.js')), false, 'Static deployment must not emit a Worker script.');

console.log(
  `Verified ${actualHtml.size} HTML routes, ${pagefindFragments.length} search fragments, ` +
    `${sitemapUrls.size} sitemap URLs, ${postSources.length} Blog articles, and ${expectedOgImages.length} OG images.`,
);
