// Small helpers for shot files (scripts/docs/shots/*.mjs).

/** The smallest element that contains every one of `texts`: a card, a row, a dialog. */
export function around(page, ...texts) {
  let loc = page.locator('div, section, form, fieldset, li, tr, aside, header, nav, [role="dialog"], [role="menu"]');
  for (const t of texts) loc = loc.filter({ hasText: t });
  return loc.last();
}

const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// CSS, not roles: an open menu or dialog hides the rest of the page from the
// accessibility tree, and the shot still needs to find things behind it.

/** A visible button or link by its exact text or aria-label. */
export function button(page, name) {
  const re = new RegExp(`^\\s*${esc(name)}\\s*$`);
  return page
    .locator('button, a, [role="button"]')
    .filter({ hasText: re })
    .or(page.locator(`button[aria-label="${name}"], a[aria-label="${name}"]`))
    .filter({ visible: true })
    .first();
}

/** A visible link whose text starts with `name` (sidebar folders and views carry counts). */
export function link(page, name) {
  return page.locator('a').filter({ hasText: new RegExp(`^\\s*${esc(name)}`) }).filter({ visible: true }).first();
}

/** An element by its aria-label. */
export function labelled(page, label) {
  return page.locator(`[aria-label="${label}"]`).first();
}

/** Text, exact by default. */
export function text(page, t, exact = true) {
  return page.getByText(t, { exact }).first();
}

/** Waits a beat for a popover, menu or route change to settle. */
export async function settle(page, ms = 350) {
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(ms);
}

/**
 * A band of rows inside `container`, full width: from the row whose label is `first`
 * to the row whose label is `last`, a little inside their edges so rings on neighbouring
 * bands keep apart. Rings around label/value rows line up this way.
 */
export async function rows(container, first, last = first) {
  return container.evaluate(
    (c, [first, last]) => {
      const cw = c.getBoundingClientRect().width;
      const row = (label) => {
        const walk = document.createTreeWalker(c, NodeFilter.SHOW_TEXT);
        for (let n = walk.nextNode(); n; n = walk.nextNode()) {
          if (n.textContent.trim() !== label) continue;
          let el = n.parentElement;
          while (el.parentElement && el.parentElement !== c && el.getBoundingClientRect().width < cw * 0.8) el = el.parentElement;
          return el.getBoundingClientRect();
        }
        throw new Error(`no row labelled ${label}`);
      };
      const a = row(first);
      const b = row(last);
      const r = c.getBoundingClientRect();
      return { x: r.x + 4, y: a.top + 3, width: r.width - 8, height: b.bottom - a.top - 6 };
    },
    [first, last],
  );
}

/**
 * The box of the nearest ancestor of `locator` at least `minWidth` wide: a list row
 * around its checkbox, a card around its title.
 */
export async function block(locator, minWidth = 300) {
  return locator.evaluate((el, minWidth) => {
    while (el.parentElement && el.getBoundingClientRect().width < minWidth) el = el.parentElement;
    const r = el.getBoundingClientRect();
    return { x: r.x, y: r.y, width: r.width, height: r.height };
  }, minWidth);
}
