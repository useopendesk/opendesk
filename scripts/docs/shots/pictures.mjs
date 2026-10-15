// Shots for docs/src/content/docs/guides/profile-pictures.mdx: your photo, contact and
// company pictures, and the workspace's logo and picture sources.
import { settle } from '../helpers.mjs';
import { framed, row } from './ai.mjs';

const TALL = { width: 1440, height: 1400 };

const drawer = (p) => p.locator('[role="dialog"]').last();
const menu = (p) => p.locator('[role="menu"]').last();
// The picture button at the top of a contact or company drawer.
const picture = (p) => drawer(p).locator('button[aria-label="Change photo"], button[aria-label="Upload photo"]').first();

async function openContact(page, name) {
  await page.getByText(name, { exact: true }).first().click();
  await settle(page, 600);
}

async function openCompany(page, name) {
  await page.getByText('Companies', { exact: true }).first().click();
  await settle(page, 500);
  await openContact(page, name);
}

const scrollTo = async (page, t) => {
  await page.getByText(t, { exact: true }).first().evaluate((el) => el.scrollIntoView({ block: 'start' }));
  await settle(page, 250);
};

export const shots = [
  {
    name: 'pictures/profile',
    path: '/settings/profile',
    clip: (p) => framed(p.getByText('Photo', { exact: true }).first()),
    pad: 20,
    highlight: [{ target: (p) => row(p, 'Photo'), label: 1, inset: { x: 10, y: 4 }, badge: 'tl' }],
  },
  {
    name: 'pictures/contact',
    path: '/contacts',
    run: async (page) => {
      await openContact(page, 'Hugo Demir');
      await picture(page).click();
      await settle(page, 400);
    },
    clip: (p) => [drawer(p).getByText('Contact', { exact: true }).first(), picture(p), menu(p), drawer(p).locator('button', { hasText: 'Edit' }).first()],
    pad: 20,
    highlight: [
      { target: picture, label: 1, inset: 3, badge: 'tl' },
      { target: (p) => [menu(p).getByRole('menuitem').first(), menu(p).getByRole('menuitem').last()], label: 2, inset: 3, badge: 'r' },
    ],
  },
  {
    name: 'pictures/company',
    path: '/contacts',
    run: async (page) => {
      await openCompany(page, 'Tidewater Media');
      await picture(page).click();
      await settle(page, 400);
    },
    clip: (p) => [drawer(p).getByText('Company', { exact: true }).first(), picture(p), menu(p), drawer(p).locator('button', { hasText: 'Edit' }).first()],
    pad: 20,
    highlight: [
      { target: picture, label: 1, inset: 3, badge: 'tl' },
      { target: (p) => menu(p).getByText(/^From /).first(), label: 2, inset: 5, badge: 'r' },
    ],
  },
  {
    name: 'pictures/workspace',
    path: '/settings/workspace',
    viewport: TALL,
    clip: (p) => framed(p.getByText('Logo', { exact: true }).first()),
    pad: 12,
    highlight: [{ target: (p) => row(p, 'Logo'), label: 1, inset: { x: 10, y: 4 }, badge: 'tl' }],
  },
  {
    name: 'pictures/help-center',
    path: '/settings/workspace',
    viewport: TALL,
    clip: (p) => framed(p.getByText('Help center logo', { exact: true }).first()),
    pad: 12,
    highlight: [
      { target: (p) => row(p, 'Help center logo'), label: 1, inset: { x: 10, y: -2 }, badge: 'tl' },
      { target: (p) => row(p, 'Favicon'), label: 2, inset: { x: 10, y: -2 }, badge: 'tl' },
    ],
  },
  {
    name: 'pictures/sources',
    path: '/settings/workspace',
    viewport: TALL,
    run: (page) => scrollTo(page, 'Contact and company pictures'),
    clip: (p) => [p.getByText('Contact and company pictures', { exact: true }).first(), framed(p.getByText('Gravatar pictures', { exact: true }).first())],
    pad: 16,
    highlight: [
      { target: (p) => row(p, 'Gravatar pictures'), label: 1, inset: { x: 10, y: -2 }, badge: 'tl' },
      { target: (p) => row(p, 'Company logos'), label: 2, inset: { x: 10, y: -2 }, badge: 'tl' },
    ],
  },
];
