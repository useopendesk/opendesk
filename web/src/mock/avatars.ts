// Generated pictures for mock mode: portraits for people, marks for companies.
// They stand in for the processed images the API serves from /v1/images.

const data = (svg: string) => `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;

function hash(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  return h >>> 0;
}

const SKIN = ["#f1d3bd", "#e3b796", "#c99470", "#a87250", "#7d5338", "#5a3a26"];
const HAIR = ["#1f1a17", "#3b2a20", "#6b4a2f", "#a37748", "#d8b47a", "#8a8a8a", "#2b2b2b"];
const BG = ["#e4e4e4", "#d4d4d4", "#c7cdd1", "#d9d2c8", "#cfd6cc", "#d6cfd8", "#262626", "#525252"];
const SHIRT = ["#171717", "#525252", "#8a8a8a", "#3d4a5c", "#5c4a3d", "#4a5c4f", "#f4f4f4"];

/** A head-and-shoulders portrait, stable for one seed. */
export function portrait(seed: string): string {
  const h = hash(seed);
  const at = <T,>(list: readonly T[], shift: number) => list[(h >>> shift) % list.length];
  const skin = at(SKIN, 0);
  const hair = at(HAIR, 5);
  const bg = at(BG, 10);
  const shirt = at(SHIRT, 15);
  const long = (h >>> 20) % 3 === 0;
  const glasses = (h >>> 23) % 4 === 0;
  const hairBack = long ? `<path d="M76 92c0-38 22-58 52-58s52 20 52 58v74h-104z" fill="${hair}"/>` : "";
  const glassesSvg = glasses
    ? `<g fill="none" stroke="#171717" stroke-width="3"><circle cx="112" cy="104" r="11"/><circle cx="144" cy="104" r="11"/><path d="M123 104h10"/></g>`
    : "";
  return data(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><rect width="256" height="256" fill="${bg}"/>${hairBack}` +
      `<path d="M40 256c6-52 44-80 88-80s82 28 88 80z" fill="${shirt}"/><rect x="114" y="138" width="28" height="40" rx="12" fill="${skin}"/>` +
      `<ellipse cx="128" cy="104" rx="40" ry="46" fill="${skin}"/>` +
      `<path d="M86 96c0-30 18-48 42-48s42 18 42 48c-10-16-26-24-42-24s-32 8-42 24z" fill="${hair}"/>${glassesSvg}</svg>`,
  );
}

/** A simple geometric company mark on a square. */
export function logoMark(name: string): string {
  const h = hash(name);
  const dark = h % 2 === 0;
  const bg = dark ? "#171717" : "#ffffff";
  const fg = dark ? "#ffffff" : "#171717";
  const shapes = [
    `<circle cx="128" cy="128" r="56" fill="${fg}"/><circle cx="128" cy="128" r="24" fill="${bg}"/>`,
    `<path d="M128 64l64 112H64z" fill="${fg}"/>`,
    `<rect x="72" y="72" width="48" height="48" fill="${fg}"/><rect x="136" y="136" width="48" height="48" fill="${fg}"/>`,
    `<path d="M72 184V72h40l32 64V72h40v112h-40l-32-64v64z" fill="${fg}"/>`,
    `<path d="M64 128a64 64 0 0 1 128 0z" fill="${fg}"/><rect x="64" y="144" width="128" height="24" fill="${fg}"/>`,
  ];
  return data(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><rect width="256" height="256" fill="${bg}"/>${shapes[(h >>> 3) % shapes.length]}</svg>`,
  );
}

const BRAND_GLYPHS: Record<string, (c: string) => string> = {
  // A page with a margin rule and two lines of text.
  margin: (c) =>
    `<rect x="70" y="52" width="116" height="152" rx="16" fill="${c}"/><rect x="98" y="52" width="12" height="152" fill="currentColor" opacity="0.28"/>` +
    `<rect x="124" y="88" width="42" height="12" rx="6" fill="currentColor" opacity="0.5"/><rect x="124" y="116" width="34" height="12" rx="6" fill="currentColor" opacity="0.5"/>`,
  // A press sheet: a headline bar over columns.
  pressroom: (c) =>
    `<rect x="60" y="64" width="136" height="128" rx="14" fill="${c}"/><rect x="80" y="84" width="96" height="20" rx="5" fill="currentColor" opacity="0.5"/>` +
    `<rect x="80" y="116" width="42" height="56" rx="5" fill="currentColor" opacity="0.35"/><rect x="134" y="116" width="42" height="10" rx="5" fill="currentColor" opacity="0.35"/>` +
    `<rect x="134" y="139" width="42" height="10" rx="5" fill="currentColor" opacity="0.35"/><rect x="134" y="162" width="30" height="10" rx="5" fill="currentColor" opacity="0.35"/>`,
};

/**
 * A brand's logo: its mark in white on the brand colour. `inverse` draws the mark in
 * the brand colour on white, as an uploaded help center logo might be.
 */
export function brandLogo(slug: string, color: string, inverse = false): string {
  const glyph = BRAND_GLYPHS[slug];
  if (!glyph) return logoMark(slug);
  const bg = inverse ? "#ffffff" : color;
  const fg = inverse ? color : "#ffffff";
  return data(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" color="${bg}"><rect width="256" height="256" rx="56" fill="${bg}"/>${glyph(fg)}</svg>`,
  );
}

/** Reads an uploaded image into a data URL, as the API would store and serve it. */
export async function fileToUrl(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return `data:${file.type || "image/png"};base64,${btoa(bin)}`;
}
