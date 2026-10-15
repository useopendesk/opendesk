// One mark for every mail host: the Google and Microsoft glyphs, the shipped
// logos (public/logos/hosts), a server icon for generic hosts, else a lettered tile.

import { Server } from "lucide-react";
import { useEffect, useState } from "react";
import { cn } from "@/lib/cn";
import { isGoogleHost, isMicrosoftHost, mailHostImage, mailHostLabel } from "@/lib/mailHost";
import { tone } from "@/lib/tones";
import { GoogleGlyph, MicrosoftGlyph } from "./kinds";

type Size = "sm" | "md" | "lg";

const BOX: Record<Size, string> = { sm: "size-6 rounded-md", md: "size-8 rounded-md", lg: "size-10 rounded-lg" };
const MARK: Record<Size, string> = { sm: "size-3.5", md: "size-4", lg: "size-5" };
const IMG: Record<Size, string> = { sm: "size-4 rounded-[3px]", md: "size-5 rounded-[3px]", lg: "size-6 rounded" };
const LETTER: Record<Size, string> = { sm: "text-[10px]", md: "text-[12px]", lg: "text-[14px]" };

export function ProviderLogo({
  host,
  label,
  size = "md",
  className,
}: {
  /** A mail_host value, "google" / "microsoft", or "" / "other" for a generic server. */
  host: string | null | undefined;
  /** Used for the lettered tile when the host has no logo. */
  label?: string;
  size?: Size;
  className?: string;
}) {
  const h = host ?? "";
  const src = mailHostImage(h);
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  const name = label || mailHostLabel(h);

  let mark;
  let framed = "border border-line bg-surface";
  if (h === "google" || isGoogleHost(h)) {
    mark = <GoogleGlyph className={MARK[size]} />;
    framed = tone.blue.chip;
  } else if (h === "microsoft" || isMicrosoftHost(h)) {
    mark = <MicrosoftGlyph className={MARK[size]} />;
    framed = tone.cyan.chip;
  } else if (src && !failed) {
    // Logos are drawn for a light background, so the tile stays white in dark mode.
    mark = <img src={src} alt="" draggable={false} loading="lazy" onError={() => setFailed(true)} className={cn("object-contain", IMG[size])} />;
    framed = "border border-line bg-white";
  } else if (!h || h === "other" || !name) {
    mark = <Server className={MARK[size]} aria-hidden />;
    framed = tone.indigo.chip;
  } else {
    mark = <span className={cn("font-semibold leading-none text-fg-2", LETTER[size])}>{name.slice(0, 1).toUpperCase()}</span>;
  }
  return (
    <span className={cn("inline-flex shrink-0 items-center justify-center", BOX[size], framed, className)} aria-hidden>
      {mark}
    </span>
  );
}
