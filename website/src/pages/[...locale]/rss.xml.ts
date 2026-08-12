import rss from '@astrojs/rss';
import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import type { Locale } from '@/i18n/ui';
import {
  getHtmlLang,
  getLocaleRouteParam,
  getLocales,
  getPostHref,
  getPostsHref,
  getPostSlug,
  useTranslations,
} from '@/i18n/utils';

type Props = {
  locale: Locale;
};

export const getStaticPaths = (() =>
  getLocales().map((locale) => ({
    params: { locale: getLocaleRouteParam(locale) },
    props: { locale },
  }))) satisfies GetStaticPaths;

export const GET: APIRoute<Props> = async ({ site, props: { locale } }) => {
  if (!site) throw new Error('Astro site URL must be configured.');

  const t = useTranslations(locale);
  const posts = (await getCollection('posts'))
    .filter((post) => post.data.locale === locale)
    .sort((a, b) => b.data.date.getTime() - a.data.date.getTime());

  return rss({
    title: `Bub — ${t('posts.title')}`,
    description: t('posts.description'),
    site: new URL(getPostsHref(locale), site),
    customData: `<language>${getHtmlLang(locale)}</language>`,
    items: posts.map((post) => ({
      title: post.data.title,
      description: post.data.description,
      pubDate: post.data.date,
      link: getPostHref(locale, getPostSlug(post.id, locale)),
    })),
  });
};
