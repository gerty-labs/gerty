import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Gerty',
  description: 'Sovereign Kubernetes Right-Sizing',
  cleanUrls: true,

  themeConfig: {
    nav: [
      { text: 'Docs', link: '/getting-started' },
      { text: 'GitHub', link: 'https://github.com/gerty-labs/gerty' },
    ],

    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'Architecture', link: '/architecture' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Configuration', link: '/configuration' },
          { text: 'CLI', link: '/cli' },
        ],
      },
      {
        text: 'About',
        items: [
          { text: 'Manifesto', link: '/manifesto' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/gerty-labs/gerty' },
    ],
  },
})
