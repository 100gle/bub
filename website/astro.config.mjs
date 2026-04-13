import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  site: 'https://bub.build',
  vite: {
    plugins: [tailwindcss()],
  },
  integrations: [
    starlight({
      title: 'Bub',
      tagline: 'A common shape for agents that live alongside people.',
      logo: {
        light: './src/assets/logo-light.svg',
        dark: './src/assets/logo-dark.svg',
        replacesTitle: true,
      },
      favicon: '/favicon.svg',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/bubbuild/bub' },
      ],
      editLink: {
        baseUrl: 'https://github.com/bubbuild/bub/edit/main/website/',
      },
      customCss: ['./src/styles/custom.css'],
      defaultLocale: 'root',
      locales: {
        root: { label: 'English', lang: 'en' },
        'zh-cn': { label: '简体中文', lang: 'zh-CN' },
      },
      sidebar: [
        {
          label: 'Getting Started',
          translations: { 'zh-CN': '快速开始' },
          items: [
            { label: 'Introduction', link: '/' },
            { slug: 'features' },
          ],
        },
        {
          label: 'Core',
          translations: { 'zh-CN': '核心概念' },
          items: [
            { slug: 'architecture' },
            { slug: 'skills' },
          ],
        },
        {
          label: 'Channels',
          translations: { 'zh-CN': '频道' },
          items: [
            { slug: 'channels' },
            { slug: 'channels/cli' },
            { slug: 'channels/telegram' },
          ],
        },
        {
          label: 'Extension Guide',
          translations: { 'zh-CN': '扩展指南' },
          items: [
            { slug: 'extension-guide' },
          ],
        },
        {
          label: 'Deployment',
          translations: { 'zh-CN': '部署' },
          items: [
            { slug: 'deployment' },
          ],
        },
        {
          label: 'API Reference',
          translations: { 'zh-CN': 'API 参考' },
          items: [
            { slug: 'api' },
            { slug: 'api/hookspecs' },
            { slug: 'api/framework' },
            { slug: 'api/skills' },
            { slug: 'api/tools' },
          ],
        },
        {
          label: 'Posts',
          translations: { 'zh-CN': '博客' },
          items: [
            { slug: 'posts' },
            { slug: 'posts/2026-03-01-bub-socialized-evaluation-and-agent-partnership' },
            { slug: 'posts/2025-07-16-baby-bub-bootstrap-milestone' },
          ],
        },
      ],
    }),
  ],
});
