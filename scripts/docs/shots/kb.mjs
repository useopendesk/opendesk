// Shots for docs/src/content/docs/guides/knowledge-base.mdx: collections, articles, the
// editor, drafting from tickets and the help center per brand.
import { button, labelled, settle, text } from '../helpers.mjs';
import { framed } from './ai.mjs';

const main = (p) => p.locator('main');
const dialog = (p) => p.locator('[role="dialog"]').filter({ visible: true }).last();
const toolbar = (p) => [p.locator('button[aria-label="Heading"]').first(), p.locator('button[aria-label="Image"]').first()];

async function openArticle(page) {
  await page.getByText('PDF export fails for large notebooks', { exact: true }).first().click();
  await settle(page, 700);
}

export const shots = [
  {
    name: 'kb/overview',
    path: '/kb',
    highlight: [
      { target: (p) => button(p, 'New article'), label: 1, badge: 'bm' },
      { target: (p) => button(p, 'Draft from tickets'), label: 2, badge: 'bm' },
      { target: (p) => main(p).getByText('View help center', { exact: true }).first(), label: 3, inset: 6, badge: 'bm' },
      { target: (p) => [main(p).getByText('New collection', { exact: true }).first(), p.locator('nav[aria-label="Collections"]')], label: 4, inset: 6, badge: 'tl' },
      { target: (p) => [main(p).locator('button').filter({ hasText: 'All brands' }).first(), main(p).getByPlaceholder('Search articles')], label: 5, inset: 4, badge: 'bl' },
    ],
  },
  {
    name: 'kb/new-collection',
    path: '/kb',
    run: async (page) => {
      await page.getByText('New collection', { exact: true }).first().click();
      await settle(page, 500);
      await dialog(page).locator('input').first().fill('Mobile app');
      await dialog(page).locator('textarea').first().fill('Install the iPhone app and keep notes in sync on the go.');
    },
    clip: dialog,
    pad: 20,
    dim: true,
    highlight: [
      { target: (p) => [dialog(p).getByText('Name', { exact: true }), dialog(p).locator('input').first()], label: 1, inset: { x: 10, y: 3 }, badge: 'r' },
      { target: (p) => [dialog(p).getByText(/^Description/).first(), dialog(p).getByText('Shown under the collection title', { exact: false })], label: 2, inset: { x: 10, y: 3 }, badge: 'r' },
      { target: (p) => [dialog(p).getByText('Brand', { exact: true }), dialog(p).getByText('Which help centers show this collection', { exact: false })], label: 3, inset: { x: 10, y: 3 }, badge: 'r' },
    ],
  },
  {
    name: 'kb/editor',
    path: '/kb',
    run: openArticle,
    highlight: [
      { target: (p) => main(p).locator('button').filter({ hasText: 'Troubleshooting' }).first(), label: 1, inset: 3, badge: 'bl' },
      { target: (p) => [p.getByPlaceholder('Article title'), p.getByPlaceholder('A one-line summary', { exact: false })], label: 2, inset: 8, badge: 'tl' },
      { target: toolbar, label: 3, inset: 3, badge: 'tl' },
      { target: (p) => [text(p, 'URL'), p.getByText('Changing it breaks old links.', { exact: false }).first()], label: 4, inset: 8, badge: 'tl' },
      { target: (p) => [button(p, 'Save'), p.locator('button[aria-label="More actions"]').first()], label: 5, inset: 3, badge: 'bl' },
      { target: (p) => [main(p).getByText('Brand', { exact: true }).last(), main(p).getByText('Which help centers show this article.', { exact: true }).last()], label: 6, inset: 8, badge: 'tl' },
    ],
  },
  {
    name: 'kb/more',
    path: '/kb',
    run: async (page) => {
      await openArticle(page);
      await page.locator('button[aria-label="More actions"]').first().click();
      await settle(page, 400);
    },
    clip: (p) => [button(p, 'Save'), p.locator('[role="menu"]').last()],
    pad: 22,
    highlight: [{ target: (p) => p.locator('[role="menu"]').last().getByRole('menuitem', { name: 'Unpublish' }), label: 1, inset: 2, badge: 'r' }],
  },
  {
    name: 'kb/draft',
    path: '/kb',
    run: async (page) => {
      await page.getByText('Draft from tickets', { exact: true }).first().click();
      await settle(page, 700);
    },
    clip: dialog,
    pad: 20,
    dim: true,
    highlight: [
      { target: (p) => dialog(p).getByText('Add a ticket', { exact: true }).locator('xpath=..'), label: 1, inset: 3, badge: 'r' },
      { target: (p) => dialog(p).locator('input').last(), label: 2, inset: 3, badge: 'r' },
      { target: (p) => dialog(p).locator('button').filter({ hasText: 'Draft article' }).first(), label: 3, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'kb/help-center',
    path: '/help/margin',
    viewport: { width: 1440, height: 1060 },
    highlight: [
      { target: (p) => p.getByPlaceholder('Search articles'), label: 1, inset: 4, badge: 'l' },
      { target: (p) => [p.getByText('Topics', { exact: true }).first(), framed(p.getByText('Fixes for common problems', { exact: false }).first()), framed(p.getByText('Connect Margin with the tools', { exact: false }).first())], label: 2, inset: 10, badge: 'l' },
      { target: (p) => [p.getByText('Popular articles', { exact: true }).first(), p.getByText('Notebooks hold your notes.', { exact: false }).first(), p.getByText('Jul 15', { exact: true }).first()], label: 3, inset: 10, badge: 'l' },
    ],
  },
  {
    name: 'kb/brand-help-center',
    path: '/settings/brands',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await main(page).locator('a[href*="/settings/brands/"]').filter({ hasText: '/help/margin' }).first().click();
      await settle(page, 500);
      await page.getByRole('tab', { name: /^\s*Help center/ }).click();
      await settle(page);
    },
    clip: (p) => [main(p).getByText('Brands', { exact: true }).first(), main(p).getByText('Articles', { exact: true }).last().locator('xpath=..')],
    pad: 24,
    highlight: [
      { target: (p) => p.getByRole('tab', { name: /^\s*Help center/ }), label: 1, inset: 3, badge: 'bl' },
      { target: (p) => labelled(p, 'Published'), label: 2, badge: 'l' },
      { target: (p) => main(p).getByText('https://app.opendesk.sh/help/margin', { exact: true }).locator('xpath=..'), label: 3, inset: 7, badge: 'l' },
      { target: (p) => [main(p).getByText('Articles', { exact: true }).last(), main(p).getByText('Write and publish articles', { exact: false }).last(), main(p).getByText('Knowledge base', { exact: true }).last()], label: 4, inset: { x: 12, y: 10 }, badge: 'l' },
    ],
  },
];
