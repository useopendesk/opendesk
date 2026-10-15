/**
 * The painted backdrops in src/assets/backdrops, as responsive sources. Backdrop
 * renders them, and Layout preloads the hero's from <head> with the same URLs, so
 * the download starts before the body is parsed. Each also carries a ~400-byte
 * blurred copy, inlined as the image's own background, so the frame shows the
 * painting the moment the HTML arrives and the real file sharpens it.
 */
import { getImage } from 'astro:assets';
import sharp from 'sharp';
import path from 'node:path';

// Each painting's sky colour, for the instant before even the inline copy paints.
const TINT: Record<string, string> = {
  roadside: '#13348c',
  'sky-fields': '#2f63b4',
  'big-cloud': '#3466b0',
  lake: '#3f6fa8',
  meadow: '#2c5fa8',
  'green-valley': '#2453c2',
  coast: '#1f4fc4',
  plains: '#2251d0',
  pines: '#2a5fb6',
  'golden-field': '#e9c88c',
  'golden-forest': '#f2d48e',
  pampas: '#f0d2c0',
  'pink-valley': '#f1cdb6',
  'pampas-sun': '#f0cdbd',
  terraces: '#e9dca8',
  'blue-ridges': '#0e5392',
  'cumulus-fields': '#7f9fb8',
  tropical: '#2751a3',
  'cypress-lake': '#68abd7',
  desert: '#0734a9',
  'autumn-hills': '#feebac',
  'rust-valley': '#fed59e',
  savanna: '#fece9d',
  sunflowers: '#fefabc',
  'red-mountains': '#fbcca5',
  'linen-clay': '#e9ddd2',
  'brush-cyan': '#8fdcef',
  'still-lake': '#c9c4c4',
  'window-light': '#e2d6c6',
  'mist-forest': '#d6e6e2',
};

const images = import.meta.glob<{ default: ImageMetadata }>('../assets/backdrops/*.webp', { eager: true });
const WIDTHS = [480, 800, 1280, 1920, 2560];

export interface BackdropSources {
  avif: string;
  webp: string;
  src: string;
  width: number;
  height: number;
  placeholder: string;
}

// AVIF at 45 keeps the brushwork at about three quarters of WebP's size at 62.
const AVIF_QUALITY = 45;
const WEBP_QUALITY = 62;

const cache = new Map<string, Promise<BackdropSources>>();

export function backdrop(name: string): Promise<BackdropSources> {
  let hit = cache.get(name);
  if (!hit) {
    hit = build(name);
    cache.set(name, hit);
  }
  return hit;
}

async function build(name: string): Promise<BackdropSources> {
  const image = images[`../assets/backdrops/${name}.webp`]?.default;
  if (!image) throw new Error(`Backdrop: no painting named "${name}" in src/assets/backdrops`);
  const widths = [...WIDTHS.filter((w) => w < image.width), image.width];
  const [avif, webp, tiny] = await Promise.all([
    getImage({ src: image, format: 'avif', widths, quality: AVIF_QUALITY }),
    getImage({ src: image, format: 'webp', widths, quality: WEBP_QUALITY }),
    sharp(path.join(process.cwd(), 'src/assets/backdrops', `${name}.webp`)).resize(40).webp({ quality: 55 }).toBuffer(),
  ]);
  return {
    avif: avif.srcSet.attribute,
    webp: webp.srcSet.attribute,
    // Only browsers without <picture> read this: the 1280 WebP, never the original.
    src: webp.srcSet.values.find((v) => v.descriptor === '1280w')?.url ?? webp.srcSet.values.at(-1)?.url ?? webp.src,
    width: image.width,
    height: image.height,
    placeholder: `background:${TINT[name] ?? 'var(--surface)'} url(data:image/webp;base64,${tiny.toString('base64')}) center/cover no-repeat;`,
  };
}
