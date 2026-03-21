import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://alphawavesystems.github.io',
  base: '/flutter-probe',
  integrations: [
    starlight({
      title: 'FlutterProbe',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/AlphaWaveSystems/flutter-probe' },
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
      ],
    }),
  ],
});
