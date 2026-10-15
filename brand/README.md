# opendesk brand

Monochrome, geometric and quiet. The mark, "Module", is a 2×2 grid with one piece reshaped into a circle: modular, extensible, open to plugins.

Figma source: https://www.figma.com/design/JfEIfmFuwLsDJCSRXOUQGH/Branding

## Files

| Path | Use |
| --- | --- |
| `logo/lockup.svg`, `logo/lockup-white.svg` | Primary logo (mark + wordmark) |
| `logo/stacked*.svg` | Square spaces, splash screens |
| `logo/mark*.svg` | Mark on its own (avatars, small sizes) |
| `logo/wordmark*.svg` | Wordmark on its own, outlined |
| `icons/favicon.svg` | Favicon that adapts to light and dark |
| `icons/icon-*.png`, `icons/apple-touch-icon.png` | PWA, browser and app icons |
| `social/og.jpg` | Open Graph / GitHub social preview (1920×1008), from `pnpm og` in `site/` |
| `social/avatar.jpg`, `social/avatar-light.jpg` | Profile pictures (1024×1024, square): white mark on ink, ink mark on white. Safe in a circle crop |
| `social/avatar-painted.jpg` | Profile picture: the mark on a frosted tile over the hero painting, to match the social card |
| `tokens.css`, `tokens.json` | Colours and type tokens |
| `logo/concepts/` | The 10 explored directions, kept for reference |

Regenerate the SVGs with `node scripts/build-brand.mjs`, then the social card and profile pictures with `pnpm og` in `site/`.

## Rules

- **Colour:** ink `#0A0A0A` on paper `#FFFFFF`, or the reverse. No gradients, tints, outlines or shadows.
- **Clear space:** at least ¼ of the mark's height on every side. Never close or change the gaps between modules.
- **Minimum size:** mark 16px; lockup 80px wide.
- **Type:** Geist SemiBold at −4% tracking for headlines, Geist Regular for body, Geist Mono for code and labels.
- **Name:** always lowercase: `opendesk`. Use `opendesk.sh` only when you mean the domain.

## Palette

| Name | Hex | Use |
| --- | --- | --- |
| Ink | `#0A0A0A` | Logo, primary text, primary buttons |
| Coal | `#171717` | Dark surfaces |
| Charcoal | `#262626` | Raised dark surfaces, dark borders |
| Graphite | `#525252` | Secondary text |
| Smoke | `#8A8A8A` | Muted text, icons |
| Ash | `#E4E4E4` | Borders, dividers |
| Mist | `#F4F4F4` | Subtle fills, hover |
| Paper | `#FFFFFF` | Background |
