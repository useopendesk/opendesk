// Marks `[data-live]` blocks while they are on screen, so their loops only run
// when someone can see them.
export function initOnscreen() {
  const live = new IntersectionObserver((entries) => {
    for (const e of entries) e.target.classList.toggle('is-live', e.isIntersecting);
  });
  document.querySelectorAll('[data-live]').forEach((el) => live.observe(el));
}
