// The look builder (contracts/bots.md "BotLook"): a built face from parts, an
// emoji on a shape, or an uploaded picture. Every tile is drawn by the shared
// renderer, so what you pick is exactly what visitors see. Part grids are radio
// groups: arrow keys move and pick, like native radios.

import { Check } from "lucide-react";
import { useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { TextInput } from "@/components/ui/input";
import { ImageUpload } from "@/components/ui/image-upload";
import { Switch } from "@/components/ui/switch";
import { Segmented } from "@/components/ui/tabs";
import { Tooltip } from "@/components/ui/tooltip";
import {
  BOT_ACCESSORIES,
  BOT_BACKGROUNDS,
  BOT_EYES,
  BOT_MOUTHS,
  BOT_PALETTE,
  BOT_SHAPES,
  isEmoji,
  isHex,
  type BotLook,
} from "@/lib/botlook";
import { cn } from "@/lib/cn";
import { BotFace } from "./BotFace";

export const EMOJI_PICKS = ["🦊", "🐙", "🦉", "🐝", "🐳", "🦄", "🐼", "🐸", "🌱", "🌻", "⭐", "🔥", "💡", "🚀", "🎧", "📚", "🧭", "🧩", "🍀", "☕", "🤖", "👋", "💬", "🎈"] as const;
const ACCENTS = ["#f59e0b", "#f6b81a", "#ef4444", "#ec4899", "#8b5cf6", "#3b82f6", "#14b8a6", "#22c55e", "#2a2838", "#ffffff"] as const;

/**
 * A tile's look: the current one with one part changed (an image look previews as built).
 * Tiles drop the background so the part shows at tile size; background tiles set their own.
 */
const variant = (look: BotLook, p: Partial<BotLook>): BotLook => ({ ...look, kind: look.kind === "image" ? "built" : look.kind, background: "none", ...p });

export function LookEditor({
  look,
  onChange,
  name,
  image,
  onUpload,
  onRemoveImage,
}: {
  look: BotLook;
  onChange: (l: BotLook) => void;
  name: string;
  image: string | null;
  onUpload: (file: Blob) => Promise<void>;
  onRemoveImage?: () => Promise<void>;
}) {
  const { t } = useTranslation("bots");
  const set = (p: Partial<BotLook>) => onChange({ ...look, ...p });
  const kind = look.kind;
  return (
    <div className="flex flex-col gap-6">
      <Group label={t("look.kind")} hint={t(`look.kindHint_${kind}`)}>
        <Segmented
          value={kind}
          onChange={(k) => set(k === "emoji" ? { kind: k, emoji: look.emoji || "🦊" } : { kind: k })}
          options={[
            { value: "built", label: t("look.built") },
            { value: "emoji", label: t("look.emoji") },
            { value: "image", label: t("look.picture") },
          ]}
        />
      </Group>

      {kind === "image" && (
        <Group label={t("look.picture")} hint={t("look.pictureHint")}>
          <ImageUpload name={name} shape="round" size="lg" value={image} onChange={onUpload} onRemove={image ? onRemoveImage : undefined} />
          {!image && <p className="mt-2 text-[12px] text-fg-3">{t("look.pictureNone")}</p>}
        </Group>
      )}

      {kind === "emoji" && <EmojiField value={look.emoji} onChange={(emoji) => set({ emoji })} />}

      {kind !== "image" && (
        <>
          <PartGrid
            label={t("look.shape")}
            values={BOT_SHAPES}
            value={look.shape}
            onChange={(shape) => set({ shape })}
            render={(shape) => variant(look, { shape })}
            name={(v) => t(`shape.${v}`)}
          />
          <ColourField look={look} onChange={set} />
        </>
      )}

      {kind === "built" && (
        <>
          <PartGrid label={t("look.eyes")} values={BOT_EYES} value={look.eyes} onChange={(eyes) => set({ eyes })} render={(eyes) => variant(look, { eyes })} name={(v) => t(`eyes.${v}`)} />
          <PartGrid label={t("look.mouth")} values={BOT_MOUTHS} value={look.mouth} onChange={(mouth) => set({ mouth })} render={(mouth) => variant(look, { mouth })} name={(v) => t(`mouth.${v}`)} />
          <PartGrid
            label={t("look.accessory")}
            values={BOT_ACCESSORIES}
            value={look.accessory}
            onChange={(accessory) => set({ accessory })}
            render={(accessory) => variant(look, { accessory })}
            name={(v) => t(`accessory.${v}`)}
          />
          {look.accessory !== "none" && look.accessory !== "halo" && look.accessory !== "sprout" && (
            <Group label={t("look.accent")} hint={t("look.accentHint")}>
              <Swatches value={look.accent} options={ACCENTS} onChange={(accent) => set({ accent })} label={t("look.accent")} />
              <HexInput value={look.accent} onChange={(accent) => set({ accent })} label={t("look.accentCustom")} className="mt-2.5" />
            </Group>
          )}
          <div className="flex items-center justify-between gap-4">
            <div>
              <label htmlFor="bot-blush" className="text-[12.5px] font-medium text-fg">
                {t("look.blush")}
              </label>
              <div className="mt-0.5 text-[12px] text-fg-3">{t("look.blushHint")}</div>
            </div>
            <Switch id="bot-blush" checked={look.blush} onCheckedChange={(blush) => set({ blush })} aria-label={t("look.blush")} />
          </div>
        </>
      )}

      {kind !== "image" && (
        <PartGrid
          label={t("look.background")}
          values={BOT_BACKGROUNDS}
          value={look.background}
          onChange={(background) => set({ background })}
          render={(background) => variant(look, { background })}
          name={(v) => t(`background.${v}`)}
        />
      )}
    </div>
  );
}

function Group({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div>
      <div className="mb-2">
        <div className="text-[12.5px] font-medium text-fg">{label}</div>
        {hint && <div className="mt-0.5 text-[12px] leading-[18px] text-fg-3">{hint}</div>}
      </div>
      {children}
    </div>
  );
}

/** Arrow keys move through a radio group and pick, like native radios. */
function useRovingRadios<V extends string>(values: readonly V[], value: V, onChange: (v: V) => void) {
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    const to = e.key === "Home" ? 0 : e.key === "End" ? values.length - 1 : step ? (i + step + values.length) % values.length : -1;
    if (to < 0) return;
    e.preventDefault();
    onChange(values[to]);
    refs.current[to]?.focus();
  };
  return { refs, onKeyDown, tabIndexOf: (v: V, i: number) => (v === value || (!values.includes(value) && i === 0) ? 0 : -1) };
}

export function PartGrid<V extends string>({
  label,
  values,
  value,
  onChange,
  render,
  name,
}: {
  label: string;
  values: readonly V[];
  value: V;
  onChange: (v: V) => void;
  render: (v: V) => BotLook;
  name: (v: V) => string;
}) {
  const { refs, onKeyDown, tabIndexOf } = useRovingRadios(values, value, onChange);
  return (
    <Group label={label}>
      <div role="radiogroup" aria-label={label} className="flex flex-wrap gap-1.5">
        {values.map((v, i) => {
          const on = v === value;
          return (
            <Tooltip key={v} content={name(v)}>
              <button
                ref={(el) => {
                  refs.current[i] = el;
                }}
                type="button"
                role="radio"
                aria-checked={on}
                aria-label={name(v)}
                tabIndex={tabIndexOf(v, i)}
                onClick={() => onChange(v)}
                onKeyDown={(e) => onKeyDown(e, i)}
                className={cn(
                  "relative flex size-[52px] items-center justify-center rounded-md border bg-surface outline-none transition-[border-color,box-shadow,background-color] duration-150 focus-visible:shadow-[0_0_0_3px_var(--focus)] motion-reduce:transition-none",
                  on ? "border-fg bg-sunken shadow-[inset_0_0_0_1px_var(--fg)]" : "border-line hover:border-line-strong hover:bg-sunken/60",
                )}
              >
                <BotFace look={render(v)} size={36} />
                {on && (
                  <span className="absolute -right-1 -top-1 flex size-3.5 items-center justify-center rounded-full bg-fg text-surface">
                    <Check className="size-2.5" strokeWidth={3} />
                  </span>
                )}
              </button>
            </Tooltip>
          );
        })}
      </div>
    </Group>
  );
}

function Swatches({ value, options, onChange, label }: { value: string; options: readonly string[]; onChange: (v: string) => void; label: string }) {
  const { refs, onKeyDown, tabIndexOf } = useRovingRadios(options, value as (typeof options)[number], onChange);
  return (
    <div role="radiogroup" aria-label={label} className="flex flex-wrap gap-1.5">
      {options.map((c, i) => (
        <button
          key={c}
          ref={(el) => {
            refs.current[i] = el;
          }}
          type="button"
          role="radio"
          aria-checked={c === value}
          aria-label={c}
          tabIndex={tabIndexOf(c, i)}
          onClick={() => onChange(c)}
          onKeyDown={(e) => onKeyDown(e, i)}
          className={cn(
            "size-6 rounded-full shadow-[inset_0_0_0_1px_rgb(0_0_0/0.12)] outline-none transition-transform duration-100 hover:scale-110 focus-visible:shadow-[0_0_0_3px_var(--focus)] motion-reduce:transition-none",
            c === value && "ring-2 ring-fg ring-offset-2 ring-offset-surface",
          )}
          style={{ background: c }}
        />
      ))}
    </div>
  );
}

function ColourField({ look, onChange }: { look: BotLook; onChange: (p: Partial<BotLook>) => void }) {
  const { t } = useTranslation("bots");
  const f = look.fill;
  const grad = f.type === "gradient";
  const ids = BOT_PALETTE.map((p) => p.name);
  const current = BOT_PALETTE.find((p) => (grad ? p.light === f.color && p.deep === f.to : p.base === f.color))?.name ?? "";
  const pick = (name: string) => {
    const p = BOT_PALETTE.find((x) => x.name === name)!;
    onChange({ fill: grad ? { type: "gradient", color: p.light, to: p.deep } : { type: "solid", color: p.base, to: p.deep } });
  };
  const { refs, onKeyDown, tabIndexOf } = useRovingRadios(ids, current, pick);
  return (
    <Group label={t("look.colour")}>
      <div className="flex flex-col gap-3">
        <Segmented
          size="xs"
          value={f.type}
          onChange={(type) => {
            const p = BOT_PALETTE.find((x) => x.base === f.color || x.light === f.color || x.deep === f.to);
            if (type === "gradient") onChange({ fill: p ? { type, color: p.light, to: p.deep } : { ...f, type } });
            else onChange({ fill: p ? { type, color: p.base, to: p.deep } : { ...f, type } });
          }}
          options={[
            { value: "solid", label: t("look.solid") },
            { value: "gradient", label: t("look.gradient") },
          ]}
          className="self-start"
        />
        <div role="radiogroup" aria-label={t("look.colour")} className="flex flex-wrap gap-1.5">
          {BOT_PALETTE.map((p, i) => (
            <Tooltip key={p.name} content={t(`palette.${p.name}`)}>
              <button
                ref={(el) => {
                  refs.current[i] = el;
                }}
                type="button"
                role="radio"
                aria-checked={current === p.name}
                aria-label={t(`palette.${p.name}`)}
                tabIndex={tabIndexOf(p.name, i)}
                onClick={() => pick(p.name)}
                onKeyDown={(e) => onKeyDown(e, i)}
                className={cn(
                  "size-7 rounded-full shadow-[inset_0_0_0_1px_rgb(0_0_0/0.12)] outline-none transition-transform duration-100 hover:scale-110 focus-visible:shadow-[0_0_0_3px_var(--focus)] motion-reduce:transition-none",
                  current === p.name && "ring-2 ring-fg ring-offset-2 ring-offset-surface",
                )}
                style={{ background: grad ? `radial-gradient(circle at 35% 30%, ${p.light}, ${p.deep})` : p.base }}
              />
            </Tooltip>
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <HexInput value={f.color} onChange={(color) => onChange({ fill: { ...f, color } })} label={grad ? t("look.from") : t("look.custom")} />
          {grad && <HexInput value={f.to} onChange={(to) => onChange({ fill: { ...f, to } })} label={t("look.to")} />}
        </div>
      </div>
    </Group>
  );
}

/** A #rrggbb field with a swatch; applies once the value is a full colour. */
function HexInput({ value, onChange, label, className }: { value: string; onChange: (v: string) => void; label: string; className?: string }) {
  const [text, setText] = useState(value);
  const [prev, setPrev] = useState(value);
  if (prev !== value) {
    setPrev(value);
    setText(value);
  }
  const bad = text !== "" && !isHex(text.startsWith("#") ? text : `#${text}`);
  return (
    <label className={cn("flex items-center gap-2 text-[12px] text-fg-3", className)}>
      <span className="w-12 shrink-0">{label}</span>
      <span className="relative">
        <span className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 rounded-full shadow-[inset_0_0_0_1px_rgb(0_0_0/0.15)]" style={{ background: isHex(value) ? value : "transparent" }} />
        <TextInput
          inputSize="sm"
          value={text}
          invalid={bad}
          maxLength={7}
          spellCheck={false}
          aria-label={label}
          className="mono w-[104px] pl-7 text-[12px] uppercase"
          onChange={(e) => {
            const v = e.target.value.trim();
            setText(v);
            const hex = v.startsWith("#") ? v : `#${v}`;
            if (isHex(hex)) onChange(hex.toLowerCase());
          }}
        />
      </span>
    </label>
  );
}

function EmojiField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation("bots");
  const [text, setText] = useState(value);
  const [prev, setPrev] = useState(value);
  if (prev !== value) {
    setPrev(value);
    setText(value);
  }
  const bad = text !== "" && !isEmoji(text.trim());
  const { refs, onKeyDown, tabIndexOf } = useRovingRadios(EMOJI_PICKS, value as (typeof EMOJI_PICKS)[number], onChange);
  return (
    <Group label={t("look.emoji")} hint={t("look.emojiHint")}>
      <div className="flex flex-col gap-2.5">
        <div role="radiogroup" aria-label={t("look.emojiPicks")} className="grid grid-cols-12 gap-1">
          {EMOJI_PICKS.map((e, i) => (
            <button
              key={e}
              ref={(el) => {
                refs.current[i] = el;
              }}
              type="button"
              role="radio"
              aria-checked={e === value}
              aria-label={e}
              tabIndex={tabIndexOf(e, i)}
              onClick={() => onChange(e)}
              onKeyDown={(ev) => onKeyDown(ev, i)}
              className={cn(
                "flex size-8 items-center justify-center rounded-md text-[18px] outline-none transition-colors duration-100 focus-visible:shadow-[0_0_0_3px_var(--focus)] motion-reduce:transition-none",
                e === value ? "bg-sunken shadow-[inset_0_0_0_1.5px_var(--fg)]" : "hover:bg-hover",
              )}
            >
              {e}
            </button>
          ))}
        </div>
        <label className="flex items-center gap-2 text-[12px] text-fg-3">
          <span>{t("look.emojiOwn")}</span>
          <TextInput
            inputSize="sm"
            value={text}
            invalid={bad}
            maxLength={16}
            aria-label={t("look.emojiOwn")}
            className="w-[72px] text-center text-[15px]"
            onChange={(e) => {
              const v = e.target.value;
              setText(v);
              if (isEmoji(v.trim())) onChange(v.trim());
            }}
          />
          {bad && <span className="text-danger-fg">{t("look.emojiBad")}</span>}
        </label>
      </div>
    </Group>
  );
}
