// One mark for every product opendesk works with (integrations, ticket links,
// imports): the vendor's shipped logo (lib/appLogos), else the icon a custom server
// gave, else the product's initial in its tone.

import { useEffect, useState, type ReactNode } from "react";
import { appLogo } from "@/lib/appLogos";
import { cn } from "@/lib/cn";
import { tone, type Tone } from "@/lib/tones";

type Size = "sm" | "md" | "lg";

const BOX: Record<Size, string> = { sm: "size-5 rounded-md", md: "size-8 rounded-md", lg: "size-10 rounded-lg" };
const IMG: Record<Size, string> = { sm: "size-3", md: "size-5", lg: "size-6" };
const LETTER: Record<Size, string> = { sm: "text-[10.5px] [&_svg]:size-3", md: "text-[13.5px] [&_svg]:size-4", lg: "text-[17px] [&_svg]:size-5" };

export function AppMark({
  app,
  name,
  icon,
  tint = "teal",
  glyph,
  size = "md",
  className,
}: {
  /** A catalog provider key or import source. */
  app?: string | null;
  /** Its name, for the initial when there is no logo. */
  name: string;
  /** A picture from the API (a custom server's own icon), or null. */
  icon?: string | null;
  /** The tone of the initial. */
  tint?: Tone;
  /** Drawn instead of the initial (a file for a CSV import). */
  glyph?: ReactNode;
  size?: Size;
  className?: string;
}) {
  const src = appLogo(app) ?? (icon ? `${icon}${icon.includes("?") ? "&" : "?"}s=64` : null);
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);

  if (src && !failed) {
    return (
      // Logos are drawn for a light background, so the tile stays white in dark mode.
      <span aria-hidden className={cn("flex shrink-0 select-none items-center justify-center border border-line bg-white", BOX[size], className)}>
        <img
          src={src}
          alt=""
          draggable={false}
          loading="lazy"
          onError={() => setFailed(true)}
          className={cn("object-contain", IMG[size], !appLogo(app) && "rounded-[3px]")}
        />
      </span>
    );
  }
  return (
    <span
      aria-hidden
      className={cn(
        "flex shrink-0 select-none items-center justify-center font-semibold tracking-[-0.02em]",
        BOX[size],
        LETTER[size],
        tone[tint].chip,
        className,
      )}
    >
      {glyph ?? (name.trim()[0] ?? "?").toUpperCase()}
    </span>
  );
}
