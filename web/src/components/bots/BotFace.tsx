// A bot's face (contracts/bots.md), drawn by the shared renderer in lib/botlook.
// The same SVG the widget and the server draw: an uploaded picture shows as a
// round image; anything else is the rendered look. `state` drives the motion
// presets (thinking bob, happy hop); `blink` adds the idle blink. Reduced motion
// stills all of it (the CSS checks prefers-reduced-motion).

import { useMemo } from "react";
import type { BotRef } from "@/lib/api/types";
import { BOT_LOOK_CSS, readLook, renderBotSvg } from "@/lib/botlook";
import { cn } from "@/lib/cn";

let injected = false;
function injectCss() {
  if (injected || typeof document === "undefined") return;
  injected = true;
  const el = document.createElement("style");
  el.dataset.odBotLook = "";
  el.textContent = BOT_LOOK_CSS;
  document.head.appendChild(el);
}

export type BotState = "idle" | "thinking" | "happy";

export function BotFace({
  look,
  image,
  size = 24,
  state = "idle",
  blink = false,
  label,
  className,
}: {
  look: unknown;
  /** The uploaded picture, shown when the look's kind is "image". */
  image?: string | null;
  size?: number;
  state?: BotState;
  blink?: boolean;
  /** Accessible name; decorative (aria-hidden) when absent. */
  label?: string;
  className?: string;
}) {
  injectCss();
  const l = useMemo(() => readLook(look), [look]);
  const svg = useMemo(() => renderBotSvg(l), [l]);
  const a11y = label ? { role: "img", "aria-label": label } : { "aria-hidden": true };
  if (l.kind === "image" && image)
    return <img src={image} alt={label ?? ""} aria-hidden={label ? undefined : true} className={cn("shrink-0 rounded-full object-cover", className)} style={{ width: size, height: size }} />;
  return (
    <span
      {...a11y}
      data-bot-state={state === "idle" ? undefined : state}
      className={cn("inline-block shrink-0", blink && "odb-anim", className)}
      style={{ width: size, height: size }}
      // The renderer emits only its own constants and validated values; readLook normalises the rest.
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}

/** A message's or a live reply's bot. */
export function BotRefFace({ bot, size = 24, state, className }: { bot: Pick<BotRef, "look" | "avatar_url"> & Partial<BotRef>; size?: number; state?: BotState; className?: string }) {
  return <BotFace look={bot.look} image={bot.avatar_url} size={size} state={state} className={className} />;
}
