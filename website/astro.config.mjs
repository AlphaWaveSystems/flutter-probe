import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sitemap from '@astrojs/sitemap';

export default defineConfig({
  site: 'https://alphawavesystems.github.io',
  base: '/flutter-probe',
  integrations: [
    sitemap(),
    starlight({
      title: 'FlutterProbe',
      description: 'Open-source Flutter E2E testing framework — write tests in plain English, execute via direct widget-tree access with sub-50ms round-trips.',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/AlphaWaveSystems/flutter-probe' },
      ],
      head: [
        { tag: 'meta', attrs: { property: 'og:image', content: 'https://alphawavesystems.github.io/flutter-probe/og-image.png' } },
        { tag: 'meta', attrs: { name: 'twitter:card', content: 'summary_large_image' } },
        { tag: 'meta', attrs: { name: 'twitter:image', content: 'https://alphawavesystems.github.io/flutter-probe/og-image.png' } },
      ],
      sidebar: [
        {
          label: 'Getting Started',
          items: [
            { label: 'Installation', slug: 'getting-started/installation' },
            { label: 'Quick Start', slug: 'getting-started/quick-start' },
          ],
        },
        {
          label: 'ProbeScript',
          items: [
            { label: 'Syntax', slug: 'probescript/syntax' },
            { label: 'Recipes', slug: 'probescript/recipes' },
            { label: 'Data-Driven Tests', slug: 'probescript/data-driven' },
            { label: 'Hooks', slug: 'probescript/hooks' },
          ],
        },
        {
          label: 'Platform',
          items: [
            { label: 'Android', slug: 'platform/android' },
            { label: 'iOS', slug: 'platform/ios' },
            { label: 'App Lifecycle', slug: 'platform/app-lifecycle' },
          ],
        },
        {
          label: 'Tools',
          items: [
            { label: 'CLI Reference', slug: 'tools/cli-reference' },
            { label: 'probe-convert', slug: 'tools/probe-convert' },
            { label: 'VS Code Extension', slug: 'tools/vscode' },
            { label: 'Recording', slug: 'tools/recording' },
            { label: 'Plugins', slug: 'tools/plugins' },
          ],
        },
        {
          label: 'CI/CD',
          items: [
            { label: 'GitHub Actions', slug: 'ci-cd/github-actions' },
            { label: 'Reports', slug: 'ci-cd/reports' },
          ],
        },
        {
          label: 'Advanced',
          items: [
            { label: 'Visual Regression', slug: 'advanced/visual-regression' },
            { label: 'Self-Healing', slug: 'advanced/self-healing' },
            { label: 'Configuration', slug: 'advanced/configuration' },
            { label: 'Architecture', slug: 'advanced/architecture' },
          ],
        },
        {
          label: 'Comparisons',
          items: [
            { label: 'Flutter E2E Testing Guide', slug: 'comparisons/flutter-e2e-testing' },
            { label: 'Flutter Testing Frameworks', slug: 'comparisons/flutter-testing-framework' },
            { label: 'integration_test Alternative', slug: 'comparisons/integration-test-alternative' },
            { label: 'Patrol Alternative', slug: 'comparisons/patrol-alternative' },
            { label: 'Flutter UI Testing', slug: 'comparisons/flutter-ui-testing' },
            { label: 'Flutter Test Automation', slug: 'comparisons/flutter-test-automation' },
            { label: 'FlutterProbe vs Patrol vs integration_test', slug: 'comparisons/flutterprobe-vs-patrol-vs-integration-test' },
          ],
        },
        {
          label: 'Blog',
          items: [
            { label: 'Guide to Flutter E2E Testing', slug: 'blog/guide-to-flutter-e2e-testing' },
            { label: 'Why We Built FlutterProbe', slug: 'blog/why-we-built-flutterprobe' },
            { label: 'FlutterProbe vs Patrol vs integration_test', slug: 'blog/flutterprobe-vs-patrol-vs-integration-test' },
          ],
        },
      ],
    }),
  ],
});
