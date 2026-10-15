// The bot look renderer (contracts/bots.md "The look"): one pure function from a
// look to an SVG string, shared by the web app and the widget bundle so both draw
// a bot the same way. internal/ai/bots/render.go is its Go twin; both are checked
// against botlook.fixtures.json. Everything drawn is a filled path from the PARTS
// table below and every colour is a validated #rrggbb, so nothing a person types
// ever becomes markup (the emoji is escaped and validated).
//
// Keep this module dependency free and small: the widget ships it.

export const BOT_SHAPES = ["blob", "circle", "rounded", "squircle", "hexagon", "cloud"] as const;
export const BOT_EYES = ["dots", "ovals", "pills", "happy", "sleepy", "sparkle", "wink", "glasses", "star"] as const;
export const BOT_MOUTHS = ["none", "smile", "grin", "open", "cat", "flat", "tongue", "smirk", "tiny"] as const;
export const BOT_ACCESSORIES = ["none", "antenna", "headphones", "bow", "cap", "halo", "sprout", "crown"] as const;
export const BOT_BACKGROUNDS = ["none", "soft", "ring"] as const;
export const BOT_KINDS = ["built", "emoji", "image"] as const;

export type BotShape = (typeof BOT_SHAPES)[number];
export type BotEyes = (typeof BOT_EYES)[number];
export type BotMouth = (typeof BOT_MOUTHS)[number];
export type BotAccessory = (typeof BOT_ACCESSORIES)[number];
export type BotBackground = (typeof BOT_BACKGROUNDS)[number];
export type BotLookKind = (typeof BOT_KINDS)[number];

export interface BotFill {
  type: "solid" | "gradient";
  color: string;
  to: string;
}

export interface BotLook {
  kind: BotLookKind;
  shape: BotShape;
  fill: BotFill;
  eyes: BotEyes;
  mouth: BotMouth;
  accessory: BotAccessory;
  accent: string;
  blush: boolean;
  background: BotBackground;
  emoji: string;
}

/** Ollie, the default bot: the violet blob with two pill eyes. */
export const OLLIE_LOOK: BotLook = {
  kind: "built",
  shape: "blob",
  fill: { type: "gradient", color: "#c9bcff", to: "#4a2bc4" },
  eyes: "pills",
  mouth: "none",
  accessory: "none",
  accent: "#f59e0b",
  blush: false,
  background: "none",
  emoji: "",
};

const PARTS = {
  circle: "M32 7C45.81 7 57 18.19 57 32C57 45.81 45.81 57 32 57C18.19 57 7 45.81 7 32C7 18.19 18.19 7 32 7Z",
  rounded: "M21 7L43 7C50.73 7 57 13.27 57 21L57 43C57 50.73 50.73 57 43 57L21 57C13.27 57 7 50.73 7 43L7 21C7 13.27 13.27 7 21 7Z",
  squircle: "M32 7C51 7 57 13 57 32C57 51 51 57 32 57C13 57 7 51 7 32C7 13 13 7 32 7Z",
  hexagon: "M26.37 8.75Q32 5.5 37.63 8.75L49.75 15.75Q55.38 19 55.38 25.5L55.38 39.5Q55.38 46 49.75 49.25L37.63 56.25Q32 59.5 26.37 56.25L14.25 49.25Q8.62 46 8.62 39.5L8.62 25.5Q8.62 19 14.25 15.75Z",
  cloud: "M19.5 23.5C26.4 23.5 32 29.1 32 36C32 42.9 26.4 48.5 19.5 48.5C12.6 48.5 7 42.9 7 36C7 29.1 12.6 23.5 19.5 23.5ZM32 8.5C40.56 8.5 47.5 15.44 47.5 24C47.5 32.56 40.56 39.5 32 39.5C23.44 39.5 16.5 32.56 16.5 24C16.5 15.44 23.44 8.5 32 8.5ZM45 20.5C51.9 20.5 57.5 26.1 57.5 33C57.5 39.9 51.9 45.5 45 45.5C38.1 45.5 32.5 39.9 32.5 33C32.5 26.1 38.1 20.5 45 20.5ZM19.5 32L44.5 32C51.4 32 57 37.6 57 44.5L57 44.5C57 51.4 51.4 57 44.5 57L19.5 57C12.6 57 7 51.4 7 44.5L7 44.5C7 37.6 12.6 32 19.5 32Z",
  blob: "M32 7C46.06 7 56.74 16.9 56.74 30.7C56.74 45.28 46.06 57 31.74 57C17.94 57 7.26 45.8 7.26 31.48C7.26 17.42 17.94 7 32 7Z",
  shine_default: "M20.5 14.12C23.83 12.64 27.2 12.95 28.03 14.82C28.86 16.68 26.83 19.4 23.5 20.88C20.17 22.36 16.8 22.05 15.97 20.18C15.14 18.32 17.17 15.6 20.5 14.12Z",
  shine_cloud: "M24.87 12.4C27.99 11.27 31.01 11.74 31.64 13.45C32.26 15.16 30.24 17.47 27.13 18.6C24.01 19.73 20.99 19.26 20.36 17.55C19.74 15.84 21.76 13.53 24.87 12.4Z",
  shine_hex: "M21.45 15.59C24.38 14.03 27.44 14.07 28.3 15.68C29.15 17.29 27.48 19.86 24.55 21.41C21.62 22.97 18.56 22.93 17.7 21.32C16.85 19.71 18.52 17.14 21.45 15.59Z",
  eye_dots: "M24 26.6C25.88 26.6 27.4 28.12 27.4 30C27.4 31.88 25.88 33.4 24 33.4C22.12 33.4 20.6 31.88 20.6 30C20.6 28.12 22.12 26.6 24 26.6ZM40 26.6C41.88 26.6 43.4 28.12 43.4 30C43.4 31.88 41.88 33.4 40 33.4C38.12 33.4 36.6 31.88 36.6 30C36.6 28.12 38.12 26.6 40 26.6Z",
  eye_dots_hi: "M25.15 27.6C25.76 27.6 26.25 28.09 26.25 28.7C26.25 29.31 25.76 29.8 25.15 29.8C24.54 29.8 24.05 29.31 24.05 28.7C24.05 28.09 24.54 27.6 25.15 27.6ZM41.15 27.6C41.76 27.6 42.25 28.09 42.25 28.7C42.25 29.31 41.76 29.8 41.15 29.8C40.54 29.8 40.05 29.31 40.05 28.7C40.05 28.09 40.54 27.6 41.15 27.6Z",
  eye_ovals: "M24 25.9C25.49 25.9 26.7 27.74 26.7 30C26.7 32.26 25.49 34.1 24 34.1C22.51 34.1 21.3 32.26 21.3 30C21.3 27.74 22.51 25.9 24 25.9ZM40 25.9C41.49 25.9 42.7 27.74 42.7 30C42.7 32.26 41.49 34.1 40 34.1C38.51 34.1 37.3 32.26 37.3 30C37.3 27.74 38.51 25.9 40 25.9Z",
  eye_ovals_hi: "M24.9 27.2C25.45 27.2 25.9 27.65 25.9 28.2C25.9 28.75 25.45 29.2 24.9 29.2C24.35 29.2 23.9 28.75 23.9 28.2C23.9 27.65 24.35 27.2 24.9 27.2ZM40.9 27.2C41.45 27.2 41.9 27.65 41.9 28.2C41.9 28.75 41.45 29.2 40.9 29.2C40.35 29.2 39.9 28.75 39.9 28.2C39.9 27.65 40.35 27.2 40.9 27.2Z",
  eye_pills: "M26.6 28.35L26.6 28.35C27.82 28.35 28.8 29.33 28.8 30.55L28.8 33.45C28.8 34.67 27.82 35.65 26.6 35.65L26.6 35.65C25.38 35.65 24.4 34.67 24.4 33.45L24.4 30.55C24.4 29.33 25.38 28.35 26.6 28.35ZM37.4 28.35L37.4 28.35C38.62 28.35 39.6 29.33 39.6 30.55L39.6 33.45C39.6 34.67 38.62 35.65 37.4 35.65L37.4 35.65C36.18 35.65 35.2 34.67 35.2 33.45L35.2 30.55C35.2 29.33 36.18 28.35 37.4 28.35Z",
  eye_happy: "M18.8 31.6Q24 23.4 29.2 31.6Q29.6 32.6 28.6 32.9Q27.6 33.1 27.1 32.2Q24 27.4 20.9 32.2Q20.4 33.1 19.4 32.9Q18.4 32.6 18.8 31.6ZM34.8 31.6Q40 23.4 45.2 31.6Q45.6 32.6 44.6 32.9Q43.6 33.1 43.1 32.2Q40 27.4 36.9 32.2Q36.4 33.1 35.4 32.9Q34.4 32.6 34.8 31.6Z",
  eye_sleepy: "M18.8 28.4Q24 36.6 29.2 28.4Q29.6 27.4 28.6 27.1Q27.6 26.9 27.1 27.8Q24 32.6 20.9 27.8Q20.4 26.9 19.4 27.1Q18.4 27.4 18.8 28.4ZM34.8 28.4Q40 36.6 45.2 28.4Q45.6 27.4 44.6 27.1Q43.6 26.9 43.1 27.8Q40 32.6 36.9 27.8Q36.4 26.9 35.4 27.1Q34.4 27.4 34.8 28.4Z",
  eye_sparkle: "M24 24.6Q25.19 28.81 29.4 30Q25.19 31.19 24 35.4Q22.81 31.19 18.6 30Q22.81 28.81 24 24.6ZM40 24.6Q41.19 28.81 45.4 30Q41.19 31.19 40 35.4Q38.81 31.19 34.6 30Q38.81 28.81 40 24.6Z",
  eye_wink: "M24 26.6C25.88 26.6 27.4 28.12 27.4 30C27.4 31.88 25.88 33.4 24 33.4C22.12 33.4 20.6 31.88 20.6 30C20.6 28.12 22.12 26.6 24 26.6ZM34.8 31.6Q40 23.4 45.2 31.6Q45.6 32.6 44.6 32.9Q43.6 33.1 43.1 32.2Q40 27.4 36.9 32.2Q36.4 33.1 35.4 32.9Q34.4 32.6 34.8 31.6Z",
  eye_wink_hi: "M25.15 27.6C25.76 27.6 26.25 28.09 26.25 28.7C26.25 29.31 25.76 29.8 25.15 29.8C24.54 29.8 24.05 29.31 24.05 28.7C24.05 28.09 24.54 27.6 25.15 27.6Z",
  eye_glasses: "M24 23.2C27.76 23.2 30.8 26.24 30.8 30C30.8 33.76 27.76 36.8 24 36.8C20.24 36.8 17.2 33.76 17.2 30C17.2 26.24 20.24 23.2 24 23.2ZM24 24.9C21.18 24.9 18.9 27.18 18.9 30C18.9 32.82 21.18 35.1 24 35.1C26.82 35.1 29.1 32.82 29.1 30C29.1 27.18 26.82 24.9 24 24.9ZM40 23.2C43.76 23.2 46.8 26.24 46.8 30C46.8 33.76 43.76 36.8 40 36.8C36.24 36.8 33.2 33.76 33.2 30C33.2 26.24 36.24 23.2 40 23.2ZM40 24.9C37.18 24.9 34.9 27.18 34.9 30C34.9 32.82 37.18 35.1 40 35.1C42.82 35.1 45.1 32.82 45.1 30C45.1 27.18 42.82 24.9 40 24.9ZM31.5 28.6L32.5 28.6C33 28.6 33.4 29 33.4 29.5L33.4 29.5C33.4 30 33 30.4 32.5 30.4L31.5 30.4C31 30.4 30.6 30 30.6 29.5L30.6 29.5C30.6 29 31 28.6 31.5 28.6ZM24 28.2C25.22 28.2 26.2 29.18 26.2 30.4C26.2 31.62 25.22 32.6 24 32.6C22.78 32.6 21.8 31.62 21.8 30.4C21.8 29.18 22.78 28.2 24 28.2ZM40 28.2C41.22 28.2 42.2 29.18 42.2 30.4C42.2 31.62 41.22 32.6 40 32.6C38.78 32.6 37.8 31.62 37.8 30.4C37.8 29.18 38.78 28.2 40 28.2Z",
  eye_star: "M23.8 25.61Q24 25.1 24.2 25.61L25.04 27.79Q25.23 28.3 25.78 28.33L28.11 28.46Q28.66 28.49 28.23 28.83L26.42 30.3Q26 30.65 26.14 31.18L26.74 33.43Q26.88 33.96 26.42 33.67L24.46 32.4Q24 32.1 23.54 32.4L21.58 33.67Q21.12 33.96 21.26 33.43L21.86 31.18Q22 30.65 21.58 30.3L19.77 28.83Q19.34 28.49 19.89 28.46L22.22 28.33Q22.77 28.3 22.96 27.79ZM39.8 25.61Q40 25.1 40.2 25.61L41.04 27.79Q41.23 28.3 41.78 28.33L44.11 28.46Q44.66 28.49 44.23 28.83L42.42 30.3Q42 30.65 42.14 31.18L42.74 33.43Q42.88 33.96 42.42 33.67L40.46 32.4Q40 32.1 39.54 32.4L37.58 33.67Q37.12 33.96 37.26 33.43L37.86 31.18Q38 30.65 37.58 30.3L35.77 28.83Q35.34 28.49 35.89 28.46L38.22 28.33Q38.77 28.3 38.96 27.79Z",
  mouth_smile: "M25.8 39.4Q32 47.2 38.2 39.4Q38.6 38.8 38 38.4Q37.3 37.9 36.7 38.6Q32 44 27.3 38.6Q26.7 37.9 26 38.4Q25.4 38.8 25.8 39.4Z",
  mouth_grin: "M25.4 38.4L38.6 38.4C38.6 43.6 35.6 46.6 32 46.6C28.4 46.6 25.4 43.6 25.4 38.4Z",
  mouth_tongue: "M25.4 38.4L38.6 38.4C38.6 43.6 35.6 46.6 32 46.6C28.4 46.6 25.4 43.6 25.4 38.4Z",
  mouth_tongue_t: "M28.2 45.1C29 42.6 35 42.6 35.8 45.1C34.8 46.1 33.4 46.6 32 46.6C30.6 46.6 29.2 46.1 28.2 45.1Z",
  mouth_open: "M32 38.1C33.82 38.1 35.3 39.85 35.3 42C35.3 44.15 33.82 45.9 32 45.9C30.18 45.9 28.7 44.15 28.7 42C28.7 39.85 30.18 38.1 32 38.1Z",
  mouth_cat: "M25.6 39.6Q29 46 32 41.2Q35 46 38.4 39.6Q38.6 38.7 37.7 38.6Q37.1 38.6 36.9 39.2Q35 42.6 32.9 38.9Q32.5 38.3 32 38.3Q31.5 38.3 31.1 38.9Q29 42.6 27.1 39.2Q26.9 38.6 26.3 38.6Q25.4 38.7 25.6 39.6Z",
  mouth_flat: "M28.2 40L35.8 40C36.46 40 37 40.54 37 41.2L37 41.2C37 41.86 36.46 42.4 35.8 42.4L28.2 42.4C27.54 42.4 27 41.86 27 41.2L27 41.2C27 40.54 27.54 40 28.2 40Z",
  mouth_smirk: "M27 41.6Q33.6 44.6 38.6 38.4Q39 37.6 38.2 37.4Q37.6 37.3 37.2 37.9Q33.2 42.4 27.6 39.8Q26.9 39.6 26.6 40.3Q26.4 41.2 27 41.6Z",
  mouth_tiny: "M28.8 40.2Q32 44.4 35.2 40.2Q35.5 39.5 34.9 39.3Q34.4 39.1 34 39.6Q32 41.9 30 39.6Q29.6 39.1 29.1 39.3Q28.5 39.5 28.8 40.2Z",
  blush: "M18.6 35.3C20.64 35.3 22.3 36.33 22.3 37.6C22.3 38.87 20.64 39.9 18.6 39.9C16.56 39.9 14.9 38.87 14.9 37.6C14.9 36.33 16.56 35.3 18.6 35.3ZM45.4 35.3C47.44 35.3 49.1 36.33 49.1 37.6C49.1 38.87 47.44 39.9 45.4 39.9C43.36 39.9 41.7 38.87 41.7 37.6C41.7 36.33 43.36 35.3 45.4 35.3Z",
  acc_antenna_stick: "M32 -1.5L32 -1.5C32.55 -1.5 33 -1.05 33 -0.5L33 7C33 7.55 32.55 8 32 8L32 8C31.45 8 31 7.55 31 7L31 -0.5C31 -1.05 31.45 -1.5 32 -1.5Z",
  acc_antenna_ball: "M32 -6.4C33.88 -6.4 35.4 -4.88 35.4 -3C35.4 -1.12 33.88 0.4 32 0.4C30.12 0.4 28.6 -1.12 28.6 -3C28.6 -4.88 30.12 -6.4 32 -6.4Z",
  acc_antenna_hi: "M30.9 -5.15C31.48 -5.15 31.95 -4.68 31.95 -4.1C31.95 -3.52 31.48 -3.05 30.9 -3.05C30.32 -3.05 29.85 -3.52 29.85 -4.1C29.85 -4.68 30.32 -5.15 30.9 -5.15Z",
  acc_headphones_band: "M5 32C5 -7.5 59 -7.5 59 32L55.8 32C55.8 -3.2 8.2 -3.2 8.2 32Z",
  acc_headphones_cups: "M6 24L6.4 24C8.61 24 10.4 25.79 10.4 28L10.4 37C10.4 39.21 8.61 41 6.4 41L6 41C3.79 41 2 39.21 2 37L2 28C2 25.79 3.79 24 6 24ZM57.6 24L58 24C60.21 24 62 25.79 62 28L62 37C62 39.21 60.21 41 58 41L57.6 41C55.39 41 53.6 39.21 53.6 37L53.6 28C53.6 25.79 55.39 24 57.6 24Z",
  acc_bow: "M46 9.5C40.5 2.5 35.6 4.6 36.4 9.8C37 14 41 15 46 9.5ZM46 9.5C51.5 2.5 56.4 4.6 55.6 9.8C55 14 51 15 46 9.5Z",
  acc_bow_knot: "M46 6.8C47.49 6.8 48.7 8.01 48.7 9.5C48.7 10.99 47.49 12.2 46 12.2C44.51 12.2 43.3 10.99 43.3 9.5C43.3 8.01 44.51 6.8 46 6.8Z",
  acc_cap_dome: "M12.5 17.5C12.5 0.5 51.5 0.5 51.5 17.5Z",
  acc_cap_brim: "M10.5 17.5C10.5 13.6 53.5 13.6 53.5 17.5C53.5 22.4 10.5 22.4 10.5 17.5Z",
  acc_cap_button: "M32 2.3C33.05 2.3 33.9 3.15 33.9 4.2C33.9 5.25 33.05 6.1 32 6.1C30.95 6.1 30.1 5.25 30.1 4.2C30.1 3.15 30.95 2.3 32 2.3Z",
  acc_halo: "M32 -3.3C39.18 -3.3 45 -1.6 45 0.5C45 2.6 39.18 4.3 32 4.3C24.82 4.3 19 2.6 19 0.5C19 -1.6 24.82 -3.3 32 -3.3ZM32 -1.6C26.48 -1.6 22 -0.66 22 0.5C22 1.66 26.48 2.6 32 2.6C37.52 2.6 42 1.66 42 0.5C42 -0.66 37.52 -1.6 32 -1.6Z",
  acc_sprout_stem: "M31.3 8.5C31 4.5 31.6 1.4 32.7 -1L34 -0.4C33.1 1.8 32.8 4.8 33.1 8.5Z",
  acc_sprout_leaves: "M32.5 1.2C28.2 -4.4 22.4 -2.6 21.6 0.4C25.2 3.4 29.6 3.8 32.5 1.2ZM33.2 -0.4C36 -6.4 42.6 -6.6 43.8 -3.8C41 -0.4 36.6 0.8 33.2 -0.4Z",
  acc_crown: "M18.9 12Q18 12 18.11 11.11L19.39 0.39Q19.5 -0.5 20.15 0.13L25.15 4.97Q25.8 5.6 26.32 4.86L31.48 -2.46Q32 -3.2 32.52 -2.46L37.68 4.86Q38.2 5.6 38.85 4.97L43.85 0.13Q44.5 -0.5 44.61 0.39L45.89 11.11Q46 12 45.1 12Z",
  acc_crown_gems: "M32 4.4C32.99 4.4 33.8 5.21 33.8 6.2C33.8 7.19 32.99 8 32 8C31.01 8 30.2 7.19 30.2 6.2C30.2 5.21 31.01 4.4 32 4.4ZM24.6 6.9C25.32 6.9 25.9 7.48 25.9 8.2C25.9 8.92 25.32 9.5 24.6 9.5C23.88 9.5 23.3 8.92 23.3 8.2C23.3 7.48 23.88 6.9 24.6 6.9ZM39.4 6.9C40.12 6.9 40.7 7.48 40.7 8.2C40.7 8.92 40.12 9.5 39.4 9.5C38.68 9.5 38.1 8.92 38.1 8.2C38.1 7.48 38.68 6.9 39.4 6.9Z",
  bg_soft: "M32 0C49.67 0 64 14.33 64 32C64 49.67 49.67 64 32 64C14.33 64 0 49.67 0 32C0 14.33 14.33 0 32 0Z",
  bg_ring: "M32 0.4C49.45 0.4 63.6 14.55 63.6 32C63.6 49.45 49.45 63.6 32 63.6C14.55 63.6 0.4 49.45 0.4 32C0.4 14.55 14.55 0.4 32 0.4ZM32 2.2C15.54 2.2 2.2 15.54 2.2 32C2.2 48.46 15.54 61.8 32 61.8C48.46 61.8 61.8 48.46 61.8 32C61.8 15.54 48.46 2.2 32 2.2Z",
} as const;

const INK_LIGHT = "#ffffff";
const INK_DARK = "#2a2838";
const STICK = "#3f3a55";
const GOLD = "#f6b81a";
const LEAF_DARK = "#2f9e5b";
const LEAF = "#4cc27a";
const BLUSH = "#ff7a9a";
const TONGUE = "#ff7a93";
const EMOJI_FONT = "Apple Color Emoji,Segoe UI Emoji,Noto Color Emoji,sans-serif";

// The face's transform: a background shrinks it, an accessory makes room above it.
const TRANSFORMS: Record<BotBackground, [string, string]> = {
  none: ["", "matrix(.9 0 0 .9 3.2 6.6)"],
  soft: ["matrix(.8 0 0 .8 6.4 6.4)", "matrix(.72 0 0 .72 8.96 11.68)"],
  ring: ["matrix(.78 0 0 .78 7.04 7.04)", "matrix(.702 0 0 .702 9.536 12.188)"],
};

const HEX = /^#[0-9a-f]{6}$/;
const part = (k: string): string => (PARTS as Record<string, string>)[k] ?? "";

/** True for a lowercase or uppercase #rrggbb colour. */
export const isHex = (s: unknown): s is string => typeof s === "string" && HEX.test(s.toLowerCase());

function rgb(hex: string): [number, number, number] {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

/** The colour the eyes and mouth are drawn in: white on dark fills, ink on light ones. */
export function inkFor(fill: BotFill): string {
  let [r, g, b] = rgb(fill.color);
  if (fill.type === "gradient") {
    const [r2, g2, b2] = rgb(fill.to);
    r = Math.floor((r + r2) / 2);
    g = Math.floor((g + g2) / 2);
    b = Math.floor((b + b2) / 2);
  }
  return 299 * r + 587 * g + 114 * b >= 165000 ? INK_DARK : INK_LIGHT;
}

const path = (d: string, fill?: string, opacity?: string) =>
  `<path d="${d}"${fill ? ` fill="${fill}"` : ""}${opacity ? ` opacity="${opacity}"` : ""}/>`;

const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");

function eyes(look: BotLook, ink: string): string {
  const k = look.eyes;
  let out = path(part(`eye_${k}`));
  if (ink === INK_DARK && (k === "dots" || k === "ovals" || k === "wink")) out += path(part(`eye_${k}_hi`), "#fff");
  return `<g class="odb-eyes" fill="${ink}">${out}</g>`;
}

function mouth(look: BotLook, ink: string): string {
  if (look.mouth === "none") return "";
  let out = path(part(`mouth_${look.mouth}`));
  if (look.mouth === "tongue") out += path(PARTS.mouth_tongue_t, TONGUE);
  return `<g class="odb-mouth" fill="${ink}">${out}</g>`;
}

function accessory(look: BotLook): string {
  const a = look.accent;
  switch (look.accessory) {
    case "antenna":
      return path(PARTS.acc_antenna_stick, STICK) + path(PARTS.acc_antenna_ball, a) + path(PARTS.acc_antenna_hi, "#fff", ".6");
    case "headphones":
      return path(PARTS.acc_headphones_band, STICK) + path(PARTS.acc_headphones_cups, a);
    case "bow":
      return path(PARTS.acc_bow, a) + path(PARTS.acc_bow_knot, a) + path(PARTS.acc_bow_knot, "#000", ".18");
    case "cap":
      return path(PARTS.acc_cap_dome, a) + path(PARTS.acc_cap_brim, a) + path(PARTS.acc_cap_brim, "#000", ".18") + path(PARTS.acc_cap_button, a);
    case "halo":
      return path(PARTS.acc_halo, GOLD);
    case "sprout":
      return path(PARTS.acc_sprout_stem, LEAF_DARK) + path(PARTS.acc_sprout_leaves, LEAF);
    case "crown":
      return path(PARTS.acc_crown, GOLD) + path(PARTS.acc_crown_gems, a);
    default:
      return "";
  }
}

/**
 * The look as an SVG string (viewBox 0 0 64 64, no size: the container sizes it).
 * An "image" look renders as its built look; callers show the uploaded picture
 * instead when there is one.
 */
export function renderBotSvg(look: BotLook): string {
  const fill = look.fill;
  const color = fill.color.toLowerCase();
  const to = fill.to.toLowerCase();
  const deep = fill.type === "gradient" ? to : color;
  const emoji = look.kind === "emoji" && look.emoji !== "";
  const withAcc = !emoji && look.accessory !== "none";
  const [plain, roomy] = TRANSFORMS[look.background];
  const transform = withAcc ? roomy : plain;

  let defs = "";
  let body = color;
  if (fill.type === "gradient") {
    const id = `odb-g-${color.slice(1)}-${to.slice(1)}`;
    defs = `<defs><radialGradient id="${id}" gradientUnits="userSpaceOnUse" cx="22" cy="16" r="50"><stop offset="0" stop-color="${color}"/><stop offset="1" stop-color="${to}"/></radialGradient></defs>`;
    body = `url(#${id})`;
  }

  let bg = "";
  if (look.background === "soft") bg = path(PARTS.bg_soft, deep, ".16");
  else if (look.background === "ring") bg = path(PARTS.bg_ring, deep);

  let face = `<path class="odb-body" d="${PARTS[look.shape]}" fill="${body}"/>`;
  if (emoji) {
    face += `<text x="32" y="33.5" text-anchor="middle" dominant-baseline="central" font-size="31" font-family="${EMOJI_FONT}">${esc(look.emoji)}</text>`;
  } else {
    const shine = look.shape === "cloud" ? PARTS.shine_cloud : look.shape === "hexagon" ? PARTS.shine_hex : PARTS.shine_default;
    const ink = inkFor({ type: fill.type, color, to });
    face += path(shine, "#fff", ".45");
    if (look.blush) face += path(PARTS.blush, BLUSH, ".5");
    face += eyes(look, ink) + mouth(look, ink);
    if (withAcc) face += `<g class="odb-acc">${accessory(look)}</g>`;
  }
  const inner = transform ? `<g transform="${transform}">${face}</g>` : face;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" class="odb">${defs}${bg}<g class="odb-bob">${inner}</g></svg>`;
}

/**
 * Motion for rendered looks: an idle blink (on a host with class odb-anim), a
 * thinking bob and a happy bounce (data-bot-state on any ancestor). Nothing moves
 * under prefers-reduced-motion.
 */
export const BOT_LOOK_CSS = `.odb{display:block;width:100%;height:100%;overflow:visible}
.odb .odb-bob,.odb .odb-eyes{transform-box:fill-box;transform-origin:50% 60%}
.odb-anim .odb-eyes{animation:odb-blink 5.6s ease-in-out infinite}
[data-bot-state=thinking] .odb-bob{animation:odb-bob 1s ease-in-out infinite}
[data-bot-state=happy] .odb-bob{animation:odb-hop .7s cubic-bezier(.34,1.56,.64,1)}
@keyframes odb-blink{0%,45%,49%,100%{transform:scaleY(1)}47%{transform:scaleY(.1)}}
@keyframes odb-bob{0%,100%{transform:translateY(0)}50%{transform:translateY(-7%)}}
@keyframes odb-hop{0%,100%{transform:translateY(0) scale(1)}35%{transform:translateY(-12%) scale(1.06,.94)}70%{transform:translateY(0) scale(.97,1.03)}}
@media (prefers-reduced-motion:reduce){.odb *{animation:none!important}}`;

// ── Validation (mirrors the server; the mock and the builder use it) ─────────────

const one = <T extends string>(list: readonly T[], v: unknown): v is T => typeof v === "string" && (list as readonly string[]).includes(v);

/** True when s is one emoji (a short run of pictographs, modifiers and joiners). */
export function isEmoji(s: unknown): s is string {
  if (typeof s !== "string" || s === "") return false;
  const cps = Array.from(s);
  if (cps.length > 10 || new TextEncoder().encode(s).length > 32) return false;
  let pict = false;
  for (const ch of cps) {
    const c = ch.codePointAt(0)!;
    const p =
      (c >= 0x1f000 && c <= 0x1faff) || (c >= 0x2600 && c <= 0x27bf) || (c >= 0x2300 && c <= 0x23ff) || (c >= 0x2b00 && c <= 0x2bff) ||
      (c >= 0x2190 && c <= 0x21ff) || (c >= 0x25a0 && c <= 0x25ff) || c === 0x3030 || c === 0x303d || c === 0x3297 || c === 0x3299 ||
      c === 0xa9 || c === 0xae || c === 0x203c || c === 0x2049 || c === 0x2122 || c === 0x2139 || c === 0x24c2;
    const mod = c === 0x200d || c === 0xfe0f || c === 0x20e3 || (c >= 0xe0020 && c <= 0xe007f) || (c >= 0x30 && c <= 0x39) || c === 0x23 || c === 0x2a;
    if (!p && !mod) return false;
    pict ||= p || c === 0x20e3;
  }
  return pict;
}

/** Why a look is refused (the server's message), or null when it is valid. */
export function lookProblem(v: unknown): string | null {
  if (!v || typeof v !== "object" || Array.isArray(v)) return "look must be an object";
  const o = v as Record<string, unknown>;
  const keys = ["kind", "shape", "fill", "eyes", "mouth", "accessory", "accent", "blush", "background", "emoji"];
  for (const k of Object.keys(o)) if (!keys.includes(k)) return `look.${k} is not a look setting`;
  for (const k of keys) if (!(k in o)) return `look.${k} is required`;
  if (!one(BOT_KINDS, o.kind)) return "look.kind must be built, emoji or image";
  if (!one(BOT_SHAPES, o.shape)) return "look.shape is not a shape";
  if (!one(BOT_EYES, o.eyes)) return "look.eyes is not an eye style";
  if (!one(BOT_MOUTHS, o.mouth)) return "look.mouth is not a mouth style";
  if (!one(BOT_ACCESSORIES, o.accessory)) return "look.accessory is not an accessory";
  if (!one(BOT_BACKGROUNDS, o.background)) return "look.background must be none, soft or ring";
  if (typeof o.blush !== "boolean") return "look.blush must be true or false";
  if (!isHex(o.accent)) return "look.accent must be a #rrggbb colour";
  const f = o.fill as Record<string, unknown> | null;
  if (!f || typeof f !== "object" || Array.isArray(f)) return "look.fill must be an object";
  for (const k of Object.keys(f)) if (!["type", "color", "to"].includes(k)) return `look.fill.${k} is not a fill setting`;
  if (f.type !== "solid" && f.type !== "gradient") return "look.fill.type must be solid or gradient";
  if (!isHex(f.color)) return "look.fill.color must be a #rrggbb colour";
  if (!isHex(f.to)) return "look.fill.to must be a #rrggbb colour";
  if (o.emoji !== "" && !isEmoji(o.emoji)) return "look.emoji must be one emoji";
  if (o.kind === "emoji" && o.emoji === "") return "look.emoji is required for an emoji look";
  return null;
}

/** A look read leniently (for display): unknown or missing values take Ollie's. */
export function readLook(v: unknown): BotLook {
  const o = (v && typeof v === "object" ? v : {}) as Partial<Record<keyof BotLook, unknown>>;
  const f = (o.fill && typeof o.fill === "object" ? o.fill : {}) as Partial<Record<keyof BotFill, unknown>>;
  const d = OLLIE_LOOK;
  return {
    kind: one(BOT_KINDS, o.kind) ? o.kind : d.kind,
    shape: one(BOT_SHAPES, o.shape) ? o.shape : d.shape,
    fill: {
      type: f.type === "solid" || f.type === "gradient" ? f.type : d.fill.type,
      color: isHex(f.color) ? f.color.toLowerCase() : d.fill.color,
      to: isHex(f.to) ? f.to.toLowerCase() : d.fill.to,
    },
    eyes: one(BOT_EYES, o.eyes) ? o.eyes : d.eyes,
    mouth: one(BOT_MOUTHS, o.mouth) ? o.mouth : d.mouth,
    accessory: one(BOT_ACCESSORIES, o.accessory) ? o.accessory : d.accessory,
    accent: isHex(o.accent) ? o.accent.toLowerCase() : d.accent,
    blush: typeof o.blush === "boolean" ? o.blush : d.blush,
    background: one(BOT_BACKGROUNDS, o.background) ? o.background : d.background,
    emoji: isEmoji(o.emoji) ? o.emoji : "",
  };
}

// ── Palette and randomize ──────────────────────────────────────────────────────

/** Curated colour families: a base for solid fills and a light-to-deep pair for gradients. */
export const BOT_PALETTE = [
  { name: "violet", base: "#7c5cf2", light: "#c9bcff", deep: "#4a2bc4" },
  { name: "indigo", base: "#5b6cf0", light: "#c0c8ff", deep: "#3341b8" },
  { name: "blue", base: "#2f8cf0", light: "#a9d4ff", deep: "#1c5fc0" },
  { name: "sky", base: "#22b5e3", light: "#a5ecff", deep: "#0b7fae" },
  { name: "teal", base: "#14b8a6", light: "#9ff0e3", deep: "#0b7f73" },
  { name: "green", base: "#22b35e", light: "#b3f0c8", deep: "#127a3d" },
  { name: "lime", base: "#8cc63f", light: "#dcf5b4", deep: "#5a8f1f" },
  { name: "amber", base: "#f5a524", light: "#ffe2a6", deep: "#c46f06" },
  { name: "orange", base: "#f97316", light: "#ffcfa8", deep: "#c2410c" },
  { name: "coral", base: "#f2645a", light: "#ffc4bd", deep: "#c23a32" },
  { name: "rose", base: "#ec4899", light: "#ffc2e1", deep: "#b8226c" },
  { name: "plum", base: "#a855f7", light: "#e4c8ff", deep: "#7228c7" },
  { name: "slate", base: "#64748b", light: "#cfd8e3", deep: "#3b4659" },
  { name: "ink", base: "#2b2a35", light: "#6b6880", deep: "#16151c" },
] as const;

/** A small seeded generator (mulberry32): the same seed gives the same rolls. */
export function seeded(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function weighted<T>(rand: () => number, items: readonly (readonly [T, number])[]): T {
  const total = items.reduce((s, [, w]) => s + w, 0);
  let r = rand() * total;
  for (const [v, w] of items) {
    r -= w;
    if (r < 0) return v;
  }
  return items[items.length - 1][0];
}

const pick = <T>(rand: () => number, list: readonly T[]): T => list[Math.floor(rand() * list.length) % list.length];

/**
 * A pleasing random built look: a curated colour family, a contrasting accent
 * from across the wheel, and eyes and mouths that read well together.
 */
export function randomLook(seed: number): BotLook {
  const rand = seeded(seed);
  const fi = Math.floor(rand() * BOT_PALETTE.length) % BOT_PALETTE.length;
  const fam = BOT_PALETTE[fi];
  const gradient = rand() < 0.65;
  const shape = weighted<BotShape>(rand, [["blob", 25], ["circle", 14], ["rounded", 14], ["squircle", 20], ["hexagon", 10], ["cloud", 17]]);
  // The accent sits across the wheel from the fill (the greys take any colour).
  const colours = BOT_PALETTE.length - 2;
  const ai = fi >= colours ? Math.floor(rand() * colours) % colours : (fi + 4 + Math.floor(rand() * 5)) % colours;
  const eyes = weighted<BotEyes>(rand, [["dots", 18], ["ovals", 16], ["pills", 12], ["happy", 14], ["sleepy", 6], ["sparkle", 9], ["wink", 8], ["glasses", 8], ["star", 7]]);
  const mouths: readonly (readonly [BotMouth, number])[] =
    eyes === "sleepy"
      ? [["none", 3], ["tiny", 3], ["flat", 2], ["smile", 2]]
      : eyes === "glasses"
        ? [["smile", 4], ["grin", 2], ["flat", 2], ["smirk", 2], ["tiny", 2]]
        : [["none", 4], ["smile", 6], ["grin", 4], ["open", 2], ["cat", 3], ["flat", 1], ["tongue", 2], ["smirk", 2], ["tiny", 4]];
  const mouth = weighted(rand, mouths);
  const accessory = rand() < 0.45 ? "none" : pick(rand, BOT_ACCESSORIES.slice(1));
  const warm = fam.name === "rose" || fam.name === "coral" || fam.name === "orange";
  const blush = !warm && fam.name !== "ink" && rand() < 0.45;
  const background = weighted<BotBackground>(rand, [["none", 60], ["soft", 25], ["ring", 15]]);
  return {
    kind: "built",
    shape,
    fill: gradient ? { type: "gradient", color: fam.light, to: fam.deep } : { type: "solid", color: fam.base, to: fam.deep },
    eyes,
    mouth,
    accessory,
    accent: BOT_PALETTE[ai].base,
    blush,
    background,
    emoji: "",
  };
}
