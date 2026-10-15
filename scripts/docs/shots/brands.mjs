// Shots for docs/src/content/docs/guides/brands-and-widgets.mdx.
import { around, button, labelled, settle, text } from '../helpers.mjs';

const main = (p) => p.locator('main');
const dialog = (p) => p.locator('[role="dialog"], [role="alertdialog"]').filter({ visible: true }).last();
const brandRow = (p, name) => main(p).locator('a[href*="/settings/brands/"]').filter({ hasText: `/help/${name.toLowerCase()}` }).first();
const widgetRow = (p, name) => main(p).locator('a[href*="/widgets/"]').filter({ hasText: name }).first();
const tab = (p, name) => p.getByRole('tab', { name: new RegExp(`^\\s*${name}`) });

async function openBrand(page, name, tabName) {
  await brandRow(page, name).click();
  await settle(page, 500);
  if (tabName) {
    await tab(page, tabName).click();
    await settle(page);
  }
}

export async function openWidget(page, brand, widget) {
  await openBrand(page, brand, 'Chat widgets');
  await widgetRow(page, widget).click();
  await settle(page, 900);
}

// The widget page is long: a tall window keeps every section on screen.
const tall = { width: 1440, height: 3600 };
const topics = (p) => around(p, 'When you add topics', 'Add a topic');
const offline = (p) => around(p, 'What visitors see outside business hours.', 'Reply by email when they leave');

export const shots = [
  {
    name: 'brands/overview',
    path: '/settings/brands',
    highlight: [
      { target: (p) => button(p, 'New brand'), label: 1, badge: 'l' },
      { target: (p) => [brandRow(p, 'Margin').getByText('Margin', { exact: true }), brandRow(p, 'Margin').getByText('Default', { exact: true }), brandRow(p, 'Margin').getByText('/help/margin', { exact: true }).locator('xpath=..').getByText('/help/margin')], label: 2, inset: 6 },
      { target: (p) => [brandRow(p, 'Margin').getByText('5 addresses', { exact: true }), brandRow(p, 'Margin').getByText('Help center on', { exact: true })], label: 3, inset: 6 },
      { target: (p) => p.locator('aside a, nav a').filter({ hasText: /^\s*Brands and widgets/ }).first(), label: 4, badge: 'r' },
    ],
  },
  {
    name: 'brands/new-brand',
    path: '/settings/brands',
    run: async (page) => {
      await button(page, 'New brand').click();
      await settle(page);
      await dialog(page).getByLabel('Name').fill('Inkwell');
    },
    clip: dialog,
    pad: 28,
    keepFocus: true,
    highlight: [
      { target: (p) => dialog(p).getByLabel('Name'), label: 1, badge: 'l', inset: 4 },
      { target: (p) => around(p, 'Help center path', 'Its help center lives at'), label: 2, badge: 'l', inset: 4 },
      { target: (p) => around(p, 'Colour').locator('xpath=..'), label: 3, badge: 'l', inset: 4 },
    ],
  },
  {
    name: 'brands/tabs',
    path: '/settings/brands',
    run: (page) => openBrand(page, 'Pressroom'),
    viewport: { width: 1440, height: 1200 },
    clip: (p) => [main(p).getByText('Brands', { exact: true }).last(), p.getByRole('tablist')],
    pad: 12,
    highlight: [{ target: (p) => p.getByRole('tablist'), label: 1, inset: 4 }],
  },
  {
    name: 'brands/default',
    path: '/settings/brands',
    run: (page) => openBrand(page, 'Pressroom'),
    viewport: { width: 1440, height: 1200 },
    clip: (p) => around(p, 'Make Pressroom the default', 'Delete brand'),
    pad: 24,
    highlight: [
      { target: (p) => button(p, 'Make default'), label: 1, badge: 'l' },
      { target: (p) => button(p, 'Delete brand'), label: 2, badge: 'l' },
    ],
  },
  {
    name: 'brands/addresses',
    path: '/settings/brands',
    run: (page) => openBrand(page, 'Margin', 'Addresses'),
    viewport: { width: 1440, height: 1000 },
    clip: (p) => around(p, 'Mailboxes that belong to Margin', 'partners@margin.app'),
    pad: 24,
    highlight: [
      { target: (p) => button(p, 'Add an address'), label: 1, badge: 'l' },
      { target: (p) => text(p, 'Default reply address'), label: 2, badge: 'r' },
      { target: (p) => [main(p).locator('a[href*="/settings/mailboxes/"]').filter({ hasText: 'support@margin.app' }).locator('span').getByText('Support', { exact: true }).locator('xpath=..'), main(p).locator('a[href*="/settings/mailboxes/"]').filter({ hasText: 'security@margin.app' }).locator('span').getByText('Security', { exact: true }).locator('xpath=..')], label: 3, inset: 6 },
    ],
  },
  {
    name: 'brands/widgets',
    path: '/settings/brands',
    run: (page) => openBrand(page, 'Margin', 'Chat widgets'),
    viewport: { width: 1440, height: 700 },
    clip: (p) => [main(p).getByText('/help/margin', { exact: true }).first(), around(p, 'Each website gets its own widget', 'Docs')],
    pad: 24,
    highlight: [
      { target: (p) => tab(p, 'Chat widgets'), label: 1, inset: 3 },
      { target: (p) => button(p, 'Add a widget'), label: 2, badge: 'l' },
      { target: (p) => widgetRow(p, 'Marketing site'), label: 3, inset: -2 },
    ],
  },
  {
    name: 'brands/new-widget',
    path: '/settings/brands',
    run: async (page) => {
      await openBrand(page, 'Margin', 'Chat widgets');
      await button(page, 'Add a widget').click();
      await settle(page);
      await dialog(page).getByLabel('Name').fill('Help center');
    },
    clip: dialog,
    pad: 28,
    keepFocus: true,
    highlight: [
      { target: (p) => dialog(p).getByLabel('Name'), label: 1, badge: 'l', inset: 4 },
      { target: (p) => around(p, 'Team', 'New conversations go to this team'), label: 2, badge: 'l', inset: 6 },
    ],
  },
  {
    name: 'brands/widget-secret',
    path: '/settings/brands',
    run: async (page) => {
      await openBrand(page, 'Margin', 'Chat widgets');
      await button(page, 'Add a widget').click();
      await settle(page);
      await dialog(page).getByLabel('Name').fill('Help center');
      await button(page, 'Create widget').click();
      await settle(page, 700);
    },
    clip: dialog,
    pad: 24,
    keepFocus: true,
    highlight: [{ target: (p) => dialog(p).locator('input, [data-copy], .mono').first().locator('xpath=..'), label: 1, badge: 'l', inset: 3 }],
  },
  {
    name: 'brands/help-center',
    path: '/settings/brands',
    run: (page) => openBrand(page, 'Margin', 'Help center'),
    viewport: { width: 1440, height: 1100 },
    clip: (p) => around(p, "Margin's public knowledge base.", 'The part of the link after /help/'),
    pad: 24,
    highlight: [
      { target: (p) => labelled(p, 'Published'), label: 1, badge: 'l' },
      { target: (p) => main(p).getByText('https://app.opendesk.sh/help/margin', { exact: true }).locator('xpath=..'), label: 2, badge: 'l', inset: 7 },
      { target: (p) => [main(p).getByText('/help/', { exact: true }), labelled(p, 'Path')], label: 3, badge: 'l', inset: 5 },
    ],
  },
  {
    name: 'brands/widget-overview',
    path: '/settings/brands',
    run: (page) => openWidget(page, 'Margin', 'Marketing site'),
    highlight: [
      { target: (p) => around(p, 'Its name and where its conversations go.', 'New conversations go to this team'), label: 1, inset: 10 },
      { target: (p) => p.locator('[aria-label="Live widget preview"]').locator('xpath=..'), label: 2, inset: 4 },
      { target: (p) => main(p).getByText('Live', { exact: true }).first(), label: 3, badge: 'l' },
    ],
  },
  {
    name: 'brands/widget-topics',
    path: '/settings/brands',
    viewport: tall,
    run: (page) => openWidget(page, 'Margin', 'Marketing site'),
    clip: topics,
    pad: 24,
    highlight: [
      { target: (p) => [labelled(p, 'Topic 1'), labelled(p, 'Topic 3')], label: 1, inset: 6, badge: 'l' },
      { target: (p) => [topics(p).locator('button').filter({ hasText: /^\s*Billing/ }).first(), topics(p).locator('button').filter({ hasText: /^\s*Legal/ }).first()], label: 2, inset: 6, badge: 'tr' },
      { target: (p) => button(p, 'Add a topic'), label: 3, badge: 'l' },
    ],
  },
  {
    name: 'brands/widget-offline',
    path: '/settings/brands',
    viewport: tall,
    run: (page) => openWidget(page, 'Margin', 'Marketing site'),
    clip: offline,
    pad: 24,
    highlight: [
      { target: (p) => p.locator('#widget-offline'), label: 1, badge: 'l', inset: 4 },
      { target: (p) => labelled(p, 'Reply by email when they leave'), label: 2, badge: 'l' },
    ],
  },
];
