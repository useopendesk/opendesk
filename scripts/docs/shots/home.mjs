// Shots for docs/src/content/docs/index.mdx: the hero and the small pictures on the
// guide cards. No marks: they show the product, the guides say where to click.
import { settle } from '../helpers.mjs';

async function openTicket(page) {
  await page.getByText('SSO login loops back to sign-in page').first().click();
  await settle(page, 500);
}

// The cards crop one part of a page, 16:10, from the top left of `box`.
const card = (x, y, w = 800) => ({ x, y, width: w, height: Math.round((w * 10) / 16) });

export const shots = [
  {
    name: 'home/hero',
    path: '/inbox/open',
    run: openTicket,
  },
  {
    name: 'home/inbox',
    path: '/inbox/open',
    run: openTicket,
    clip: () => card(241, 49, 720),
    pad: 0,
  },
  {
    name: 'home/mailboxes',
    path: '/settings/mailboxes',
    clip: () => card(525, 60, 840),
    pad: 0,
  },
  {
    name: 'home/chat',
    path: '/settings/widget',
    clip: () => card(980, 204, 440),
    pad: 0,
  },
  {
    name: 'home/kb',
    path: '/kb',
    clip: () => card(241, 49, 800),
    pad: 0,
  },
  {
    name: 'home/ollie',
    path: '/inbox/open',
    run: async (page) => {
      // Ollie's panel, from the button in the top bar.
      await page.locator('header').getByText('Ollie', { exact: true }).first().click();
      await settle(page, 800);
    },
    keepFocus: true,
    clip: () => card(1041, 516, 400),
    pad: 0,
  },
  {
    name: 'home/reports',
    path: '/reports',
    clip: () => card(265, 255, 762),
    pad: 0,
  },
  {
    name: 'home/automations',
    path: '/settings/automations',
    clip: () => card(525, 60, 840),
    pad: 0,
  },
  {
    name: 'home/api-keys',
    path: '/settings/api-keys',
    clip: () => card(489, 60, 760),
    pad: 0,
  },
];
