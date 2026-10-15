/*
 * Numbers in the product scenes that keep moving while the scene is on screen:
 *
 *   <span data-clock="2079">34m 39s</span>       time waited, +1s every second
 *   <span data-countdown="702" data-from="900">  time left as 11:42, -1s every second,
 *                                                 back to data-from at zero; a
 *                                                 [data-countdown-bar] in the same
 *                                                 [data-sla] row shrinks with it,
 *                                                 and a [data-countdown-ring]
 *                                                 circle drains the same way
 *   <span data-count="1284">1,284</span>          a running total, +1 now and then
 *
 * A clock inside a step that has not happened yet holds its first value, so a
 * chat that arrives again on the next round arrives fresh. Under reduced motion
 * nothing moves.
 */
const live = (el: Element) => el.closest('.stage-box')?.classList.contains('is-live') ?? true;
const waited = (s: number) => `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`;
const left = (s: number) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
const nf = new Intl.NumberFormat('en');

export function initLive() {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  const clocks = [...document.querySelectorAll<HTMLElement>('[data-clock]')].map((el) => {
    const start = Number(el.dataset.clock) || 0;
    return { el, start, n: start };
  });
  const countdowns = [...document.querySelectorAll<HTMLElement>('[data-countdown]')].map((el) => {
    const from = Number(el.dataset.from) || 900;
    const bar = el.closest('[data-sla]')?.querySelector<HTMLElement>('[data-countdown-bar]') ?? null;
    const ring = el.closest('[data-sla]')?.querySelector<SVGCircleElement>('[data-countdown-ring]') ?? null;
    return { el, from, n: Number(el.dataset.countdown) || from, bar, ring };
  });

  setInterval(() => {
    for (const c of clocks) {
      const step = c.el.closest<HTMLElement>('[data-step]');
      if (step && !step.classList.contains('on')) {
        c.n = c.start;
      } else if (live(c.el)) {
        c.n++;
      }
      c.el.textContent = waited(c.n);
    }
    for (const c of countdowns) {
      if (!live(c.el)) continue;
      c.n = c.n <= 0 ? c.from : c.n - 1;
      c.el.textContent = left(c.n);
      if (c.bar) c.bar.style.width = `${(c.n / c.from) * 100}%`;
      if (c.ring) c.ring.style.strokeDashoffset = String(Number(c.ring.getAttribute('stroke-dasharray')) * (1 - c.n / c.from));
    }
  }, 1000);

  for (const el of document.querySelectorAll<HTMLElement>('[data-count]')) {
    let n = Number(el.dataset.count) || 0;
    const tick = () => {
      if (live(el)) {
        n++;
        el.textContent = nf.format(n);
      }
      setTimeout(tick, 2200 + Math.random() * 2600);
    };
    setTimeout(tick, 2000);
  }
}
