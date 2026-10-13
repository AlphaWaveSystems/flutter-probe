import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sitemap from '@astrojs/sitemap';
import { readFileSync } from 'node:fs';

// Code blocks in ProbeScript (`probe`, `probescript`) and EBNF (`ebnf`) are highlighted with
// TextMate grammars that Expressive Code loads through `shiki.langs`. The ProbeScript grammar
// is the VS Code extension's own file, so the two stay in step; the EBNF grammar is in grammars/.
const readGrammar = (path) => JSON.parse(readFileSync(new URL(path, import.meta.url), 'utf8'));
const probescript = { ...readGrammar('../vscode/syntaxes/probe.tmGrammar.json'), name: 'probescript', aliases: ['probe'] };
const ebnf = readGrammar('./grammars/ebnf.tmLanguage.json');

export default defineConfig({
  site: 'https://flutterprobe.dev',
  integrations: [
    sitemap(),
    starlight({
      title: 'FlutterProbe',
      description: 'Flutter E2E testing framework — write tests in plain English, execute via direct widget-tree access. Free to use: source-available CLI (BSL 1.1), MIT agent.',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/AlphaWaveSystems/flutter-probe' },
      ],
      expressiveCode: {
        shiki: { langs: [probescript, ebnf] },
      },
      head: [
        { tag: 'meta', attrs: { property: 'og:image', content: 'https://flutterprobe.dev/og-image.png' } },
        { tag: 'meta', attrs: { name: 'twitter:card', content: 'summary_large_image' } },
        { tag: 'meta', attrs: { name: 'twitter:image', content: 'https://flutterprobe.dev/og-image.png' } },
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
            { label: 'Annotation-driven Tests', slug: 'probescript/annotations' },
            { label: 'Dictionary', slug: 'probescript/dictionary' },
            { label: 'Grammar (EBNF)', slug: 'probescript/grammar' },
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
            { label: 'Studio (Beta)', slug: 'tools/studio' },
            { label: 'MCP Server', slug: 'tools/mcp' },
            { label: 'Recording', slug: 'tools/recording' },
            { label: 'On-Device Agent', slug: 'tools/agent' },
            { label: 'System Dialogs', slug: 'tools/system-dialogs' },
            { label: 'Probe Cloud (preview)', slug: 'tools/probe-cloud-preview' },
            { label: 'Design-partner guide', slug: 'tools/design-partner-guide' },
            { label: 'Using Probe from AI Agents', slug: 'tools/ai-agents' },
            { label: 'AI & Local Models', slug: 'tools/ai' },
            { label: 'Plugins', slug: 'tools/plugins' },
          ],
        },
        {
          label: 'CI/CD',
          items: [
            { label: 'GitHub Actions', slug: 'ci-cd/github-actions' },
            { label: 'Reports', slug: 'ci-cd/reports' },
            { label: 'Results in other tools', slug: 'ci-cd/analytics-tools' },
          ],
        },
        {
          label: 'Advanced',
          items: [
            { label: 'Visual Regression', slug: 'advanced/visual-regression' },
            { label: 'Self-Healing', slug: 'advanced/self-healing' },
            { label: 'Testing localized apps', slug: 'advanced/multi-language' },
            { label: 'Testing against backend data', slug: 'advanced/backend-data' },
            { label: 'Timeouts, waiting and retries', slug: 'advanced/timeouts-and-retries' },
            { label: 'Performance testing', slug: 'advanced/performance' },
            { label: 'Compatibility policy (1.x)', slug: 'advanced/compatibility' },
            { label: 'Configuration', slug: 'advanced/configuration' },
            { label: 'Architecture', slug: 'advanced/architecture' },
          ],
        },
        {
          label: 'Tutorials',
          items: [
            { label: 'Write a Flutter login test', slug: 'tutorials/flutter-login-test' },
            { label: 'Run E2E tests in GitHub Actions', slug: 'tutorials/flutter-e2e-github-actions' },
            { label: 'Find and fix flaky tests', slug: 'tutorials/triage-flaky-flutter-tests' },
          ],
        },
        {
          label: 'Comparisons',
          items: [
            { label: 'Why FlutterProbe', slug: 'comparisons/why-flutterprobe' },
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
            { label: 'FlutterProbe vs Maestro: A Benchmark, Replicated', slug: 'blog/flutterprobe-vs-maestro-benchmark' },
          ],
        },
      ],
    }),
  ],
});
