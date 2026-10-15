// Shots for docs/src/content/docs/guides/billing.mdx: the Plan and billing page on Cloud.
import { button, settle } from '../helpers.mjs';
import { framed, row } from './ai.mjs';

const main = (p) => p.locator('main');
const card = (p, t, min = 400) => framed(main(p).getByText(t, { exact: true }).first(), min);
const recharge = (p) => main(p).getByRole('switch').first();
// The nearest ancestor of a heading at least `minWidth` wide: a column of the usage card.
const column = (p, t, minWidth) =>
  main(p)
    .getByText(t, { exact: true })
    .first()
    .evaluate((el, minWidth) => {
      for (let e = el; e; e = e.parentElement) {
        const r = e.getBoundingClientRect();
        if (r.width >= minWidth) return { x: r.x, y: r.y, width: r.width, height: r.height };
      }
      throw new Error('no column');
    }, minWidth);
const scrollTo = async (page, t) => {
  await page.getByText(t, { exact: true }).first().evaluate((el) => el.scrollIntoView({ block: 'start' }));
  await settle(page, 250);
};

export const shots = [
  {
    name: 'billing/overview',
    path: '/settings/billing',
    viewport: { width: 1440, height: 1120 },
    highlight: [
      { target: (p) => card(p, 'Current plan'), label: 1, inset: 4, badge: 'tl' },
      { target: (p) => card(p, 'Monthly allowance'), label: 2, inset: 4, badge: 'tl' },
      { target: (p) => [main(p).getByText('Auto-recharge', { exact: true }).first(), card(p, 'Recharge below')], label: 3, inset: 8, badge: 'tl' },
    ],
  },
  {
    name: 'billing/plan',
    path: '/settings/billing',
    clip: (p) => card(p, 'Current plan'),
    pad: 16,
    highlight: [
      { target: (p) => [main(p).getByText('Seats', { exact: true }).first(), main(p).getByText('Viewers are free.', { exact: false }).first()], label: 1, inset: 8, badge: 'bl' },
      { target: (p) => [main(p).getByText('Price', { exact: true }).first(), main(p).getByText('$18 per seat per month', { exact: true }).first()], label: 2, inset: 8, badge: 'bl' },
      { target: (p) => button(p, 'Manage billing'), label: 3, inset: 3, badge: 'bl' },
    ],
  },
  {
    name: 'billing/balance',
    path: '/settings/billing',
    viewport: { width: 1440, height: 1000 },
    clip: (p) => [main(p).getByText('AI balance', { exact: true }).first(), card(p, 'Monthly allowance')],
    pad: 16,
    highlight: [
      { target: (p) => main(p).getByText('remaining', { exact: true }).first().locator('xpath=..'), label: 1, inset: 8, badge: 'tl' },
      { target: (p) => [main(p).getByText('Monthly allowance', { exact: true }).first(), main(p).getByText('used this month', { exact: false }).first(), main(p).getByText(/^Resets /).first()], label: 2, inset: 8, badge: 'bl' },
      { target: (p) => [main(p).getByText('Allowance left', { exact: true }).first(), main(p).getByText('Never expire', { exact: true }).first()], label: 3, inset: 8, badge: 'tr' },
      { target: (p) => row(p, 'Add AI usage'), label: 4, inset: { x: 4, y: -2 }, badge: 'bl' },
    ],
  },
  {
    name: 'billing/auto-recharge',
    path: '/settings/billing',
    viewport: { width: 1440, height: 1100 },
    run: async (page) => {
      await scrollTo(page, 'Add AI usage');
      await recharge(page).click();
      await settle(page, 500);
    },
    clip: (p) => [main(p).getByText('Auto-recharge', { exact: true }).first(), card(p, 'Recharge below')],
    pad: 16,
    highlight: [
      { target: recharge, label: 1, inset: 4, badge: 'l' },
      { target: (p) => row(p, 'Recharge below'), label: 2, inset: { x: 8, y: -2 }, badge: 'tl' },
      { target: (p) => row(p, 'Recharge amount'), label: 3, inset: { x: 8, y: -2 }, badge: 'tl' },
      { target: (p) => row(p, 'Monthly limit'), label: 4, inset: { x: 8, y: -2 }, badge: 'tl' },
    ],
  },
  {
    name: 'billing/invoices',
    path: '/settings/billing',
    viewport: { width: 1440, height: 1400 },
    run: async (page) => {
      await page.getByText('Payment method and invoices', { exact: true }).first().evaluate((el) => el.scrollIntoView({ block: 'center' }));
      await settle(page, 250);
    },
    clip: (p) => [main(p).getByText('Payment method and invoices', { exact: true }).first(), card(p, 'Visa •••• 4242')],
    pad: 16,
    highlight: [
      { target: (p) => row(p, 'Visa •••• 4242'), label: 1, inset: { x: 8, y: -2 }, badge: 'tl' },
      { target: (p) => main(p).getByText('Pro subscription', { exact: true }).first().locator('xpath=ancestor::li'), label: 2, inset: { x: 8, y: -2 }, badge: 'bl' },
    ],
  },
  {
    name: 'billing/usage',
    path: '/settings/billing',
    viewport: { width: 1440, height: 1500 },
    run: (page) => scrollTo(page, 'AI usage'),
    clip: (p) => [main(p).getByText('AI usage', { exact: true }).first(), card(p, 'By model')],
    pad: 16,
    highlight: [
      { target: (p) => main(p).locator('button').filter({ hasText: 'Last 30 days' }).first(), label: 1, inset: 3, badge: 'bl' },
      { target: (p) => column(p, 'By feature', 400), label: 2, inset: -6, badge: 'tl' },
      { target: (p) => column(p, 'By member', 400), label: 3, inset: -6, badge: 'tl' },
      { target: (p) => column(p, 'By model', 800), label: 4, inset: -6, badge: 'tl' },
    ],
  },
];
