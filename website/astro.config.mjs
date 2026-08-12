// @ts-check
import { fileURLToPath } from 'node:url';
import { defineConfig } from 'astro/config';
import tailwindcss from '@tailwindcss/vite';
import starlight from '@astrojs/starlight';
import mermaid from 'astro-mermaid';

export default defineConfig({
  site: process.env.SITE_URL ?? 'https://bub.build',
  vite: {
    plugins: [tailwindcss()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
  },
  integrations: [
    // Keep this before Starlight so Mermaid registers with Astro's active
    // Markdown processor before Starlight configures syntax highlighting.
    mermaid({
      theme: 'neutral',
      autoTheme: true,
    }),
    starlight({
      title: 'Bub',
      description: 'A common shape for agents that live alongside people.',
      expressiveCode: false,
      logo: {
        light: './src/assets/bub-logo.png',
        dark: './src/assets/bub-logo-dark.png',
        alt: 'Bub',
      },
      // Use the resolved file URL so Vite's module graph reliably
      // includes global.css on every Starlight docs page in dev mode.
      // (Astro scopes CSS per-page from the import graph; with a
      // relative path Vite occasionally fails to resolve / dedupe in
      // dev, leaving the docs route unstyled even though
      // `astro build` + preview both work.)
      customCss: [fileURLToPath(new URL('./src/styles/global.css', import.meta.url))],
      favicon: '/favicon.ico',
      markdown: {
        processedDirs: ['./src/content/posts/'],
      },
      disable404Route: true,
      locales: {
        root: {
          label: 'English',
          lang: 'en',
        },
        'zh-cn': {
          label: '简体中文',
          lang: 'zh-CN',
        },
      },
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/bubbuild/bub' }],
      components: {
        Footer: './src/components/starlight/Footer.astro',
        Header: './src/components/starlight/Header.astro',
        PageFrame: './src/components/starlight/PageFrame.astro',
        SocialIcons: './src/components/starlight/SocialIcons.astro',
      },
      sidebar: [
        {
          label: 'Getting Started',
          translations: { 'zh-CN': '快速开始' },
          items: [{ autogenerate: { directory: 'docs/getting-started' } }],
        },
        {
          label: 'Concepts',
          translations: { 'zh-CN': '概念' },
          items: [{ autogenerate: { directory: 'docs/concepts' } }],
        },
        {
          label: 'Operate',
          translations: { 'zh-CN': '运行' },
          items: [{ autogenerate: { directory: 'docs/operate' } }],
        },
        {
          label: 'Build',
          translations: { 'zh-CN': '构建' },
          items: [{ autogenerate: { directory: 'docs/build' } }],
        },
        {
          label: 'Tutorials',
          translations: { 'zh-CN': '教程' },
          items: [{ autogenerate: { directory: 'docs/tutorials' } }],
        },
        {
          label: 'Reference',
          translations: { 'zh-CN': '参考' },
          items: [{ autogenerate: { directory: 'docs/reference' } }],
        },
      ],
    }),
  ],
});
