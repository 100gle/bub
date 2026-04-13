import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://bub.build',
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
        { icon: 'pypi', label: 'PyPI', href: 'https://pypi.org/project/bub' },
      ],
      editLink: {
        baseUrl: 'https://github.com/bubbuild/bub/edit/main/website/',
      },
      customCss: ['./src/styles/custom.css'],
      defaultLocale: 'en',
      locales: {
        en: { label: 'English' },
        'zh-cn': { label: '简体中文', lang: 'zh-CN' },
      },
      sidebar: [
        {
          label: 'Getting Started',
          translations: { 'zh-CN': '快速开始' },
          items: [
            { slug: 'index' },
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
            { slug: 'channels/index' },
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
            { slug: 'api/index' },
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
            { slug: 'posts/index' },
            { slug: 'posts/2026-03-01-bub-socialized-evaluation-and-agent-partnership' },
            { slug: 'posts/2025-07-16-baby-bub-bootstrap-milestone' },
          ],
        },
      ],
    }),
  ],
});
