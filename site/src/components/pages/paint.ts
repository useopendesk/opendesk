// The painting that stands for a post or release: picked from its id, so it
// stays the same on every build and on every page that shows it.
// pampas-sun is left out: it closes every page in the CTA.
export const PAINTINGS = ['lake', 'rust-valley', 'cumulus-fields', 'big-cloud', 'sky-fields'] as const;

export function paintingFor(id: string, skip: readonly string[] = []): string {
  const pool = PAINTINGS.filter((p) => !skip.includes(p));
  let h = 0;
  for (const c of id) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return pool[h % pool.length];
}

/** Rough reading time in minutes, at 220 words a minute. */
export const readingMinutes = (text: string) => Math.max(1, Math.round(text.split(/\s+/).filter(Boolean).length / 220));

/** A blog post's cover: big-cloud is left out, it is the contact page's panel. */
export const postPainting = (id: string) => paintingFor(id, ['big-cloud']);
