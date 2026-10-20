import { defineConfig } from 'vitepress'

// Where the site is published. Override with SITE_URL when deploying somewhere else.
const siteUrl = new URL(process.env.SITE_URL ?? 'https://moonshed.siddhesh.me/')
const base = siteUrl.pathname.endsWith('/') ? siteUrl.pathname : siteUrl.pathname + '/'

export default defineConfig({
  title: 'Moonshed',
  description: 'A small, self-hosted platform for deploying your apps.',
  base,
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: siteUrl.origin + base },
  head: [['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}logo.svg` }]],

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: 'Docs', link: '/docs/', activeMatch: '^/docs/(?!reference/)' },
      { text: 'Reference', link: '/docs/reference/moonshed-yaml', activeMatch: '^/docs/reference/' },
      { text: 'Changelog', link: '/changelog' },
    ],

    sidebar: {
      '/docs/': [
        {
          text: 'Getting started',
          items: [
            { text: 'What is Moonshed?', link: '/docs/' },
            { text: 'Installation', link: '/docs/installation' },
            { text: 'Deploy your first app', link: '/docs/first-deploy' },
          ],
        },
        {
          text: 'Deploying',
          items: [
            { text: 'Native Process apps', link: '/docs/native-apps' },
            { text: 'Docker apps', link: '/docs/docker-apps' },
            { text: 'Git and webhooks', link: '/docs/git' },
            { text: 'Deploying from CI', link: '/docs/ci' },
            { text: 'Deployments and rollbacks', link: '/docs/deployments' },
          ],
        },
        {
          text: 'Running',
          items: [
            { text: 'Domains and HTTPS', link: '/docs/domains' },
            { text: 'Monitoring', link: '/docs/monitoring' },
            { text: 'Backups', link: '/docs/backups' },
            { text: 'Users and security', link: '/docs/security' },
            { text: 'Upgrading', link: '/docs/upgrading' },
          ],
        },
        {
          text: 'Reference',
          items: [
            { text: 'moonshed.yaml', link: '/docs/reference/moonshed-yaml' },
            { text: 'Configuration file', link: '/docs/reference/configuration' },
            { text: 'CLI', link: '/docs/reference/cli' },
            { text: 'Runtime folder', link: '/docs/reference/runtime-folder' },
            { text: 'API', link: '/docs/reference/api' },
          ],
        },
      ],
    },

    search: { provider: 'local' },
    outline: [2, 3],

    editLink: {
      pattern: 'https://github.com/SiddheshNan/moonshed/edit/main/website/:path',
      text: 'Edit this page on GitHub',
    },

    socialLinks: [{ icon: 'github', link: 'https://github.com/SiddheshNan/moonshed' }],

    footer: {
      message: 'Released under the AGPL-3.0 license.',
    },
  },
})
