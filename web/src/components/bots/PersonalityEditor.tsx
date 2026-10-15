// How a bot sounds (contracts/bots.md "BotPersonality"): a tone, reply length,
// emoji use, its language, the lines it opens and hands off with, its email
// sign-off, what it knows and what it declines. The persona shapes voice only;
// the agent's rules and guardrails stay as they are.

import { Link } from "@tanstack/react-router";
import { Check } from "lucide-react";
import { useRef, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Textarea, TextInput } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Segmented } from "@/components/ui/tabs";
import { ChipEditor } from "@/components/agent/ChipEditor";
import { Counter } from "@/components/agent/bits";
import { topicProblem } from "@/components/agent/settings";
import type { BotEmoji, BotPersonality, BotReplyLength, BotTone } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { AI_TONE, tone } from "@/lib/tones";

export const LIMITS = { name: 40, role: 60, custom_tone: 200, instructions: 4000, greeting: 300, handoff_message: 300, sign_off: 200, topics: 20 } as const;
export const TONES: BotTone[] = ["friendly", "professional", "playful", "concise", "warm", "custom"];
const LANGUAGES = ["en", "de", "fr", "es", "it", "pt-BR", "pt", "nl", "pl", "sv", "da", "nb", "fi", "cs", "hu", "ro", "tr", "uk", "ar", "he", "ja", "ko", "zh"];
const AUTO = "auto";

/** Text a persona field may hold: no markup or control characters (newlines where allowed). */
export function textProblem(v: string, max: number, multiline = false): "long" | "markup" | null {
  if (v.trim().length > max) return "long";
  // eslint-disable-next-line no-control-regex
  if (/[<>]/.test(v) || (multiline ? /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/ : /[\u0000-\u001f\u007f]/).test(v)) return "markup";
  return null;
}

export function PersonalityEditor({
  value,
  onChange,
  language,
  onLanguage,
}: {
  value: BotPersonality;
  onChange: (p: BotPersonality) => void;
  language: string;
  onLanguage: (l: string) => void;
}) {
  const { t } = useTranslation("bots");
  const set = (p: Partial<BotPersonality>) => onChange({ ...value, ...p });
  const langName = (code: string) => {
    try {
      return new Intl.DisplayNames(["en"], { type: "language" }).of(code) ?? code;
    } catch {
      return code;
    }
  };
  const langs = LANGUAGES.includes(language) || !language ? LANGUAGES : [language, ...LANGUAGES];
  return (
    <div className="flex flex-col gap-7">
      <Block label={t("personality.tone")} hint={t("personality.toneHint")}>
        <ToneCards value={value.tone} onChange={(tone) => set({ tone })} custom={value.custom_tone} />
        {value.tone === "custom" && (
          <TextField
            id="bot-custom-tone"
            className="mt-3"
            label={t("personality.customTone")}
            value={value.custom_tone}
            max={LIMITS.custom_tone}
            placeholder={t("personality.customTonePlaceholder")}
            onChange={(custom_tone) => set({ custom_tone })}
            required
          />
        )}
      </Block>

      <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
        <Block label={t("personality.length")} hint={t(`personality.lengthHint_${value.reply_length}`)}>
          <Segmented<BotReplyLength>
            value={value.reply_length}
            onChange={(reply_length) => set({ reply_length })}
            options={(["short", "medium", "detailed"] as const).map((v) => ({ value: v, label: t(`personality.length_${v}`) }))}
          />
        </Block>
        <Block label={t("personality.emoji")} hint={t(`personality.emojiHint_${value.emoji}`)}>
          <Segmented<BotEmoji>
            value={value.emoji}
            onChange={(emoji) => set({ emoji })}
            options={(["none", "some", "lots"] as const).map((v) => ({ value: v, label: t(`personality.emoji_${v}`) }))}
          />
        </Block>
      </div>

      <Block label={t("personality.language")} hint={t("personality.languageHint")}>
        <Select
          className="w-[260px]"
          value={language || AUTO}
          onChange={(v) => onLanguage(v === AUTO ? "" : v)}
          aria-label={t("personality.language")}
          options={[{ value: AUTO, label: t("personality.languageAuto") }, ...langs.map((l) => ({ value: l, label: langName(l), hint: l }))]}
        />
      </Block>

      <TextField
        id="bot-greeting"
        area
        label={t("personality.greeting")}
        hint={t("personality.greetingHint")}
        value={value.greeting}
        max={LIMITS.greeting}
        placeholder={t("personality.greetingPlaceholder")}
        onChange={(greeting) => set({ greeting })}
      />
      <TextField
        id="bot-handoff"
        area
        label={t("personality.handoff")}
        hint={t("personality.handoffHint")}
        value={value.handoff_message}
        max={LIMITS.handoff_message}
        placeholder={t("personality.handoffPlaceholder")}
        onChange={(handoff_message) => set({ handoff_message })}
      />
      <TextField
        id="bot-signoff"
        area
        rows={2}
        label={t("personality.signOff")}
        hint={t("personality.signOffHint")}
        value={value.sign_off}
        max={LIMITS.sign_off}
        placeholder={t("personality.signOffPlaceholder")}
        onChange={(sign_off) => set({ sign_off })}
      />

      <Block label={t("personality.instructions")} hint={t("personality.instructionsHint")} htmlFor="bot-instructions">
        <Textarea
          id="bot-instructions"
          rows={7}
          value={value.instructions}
          invalid={value.instructions.length > LIMITS.instructions}
          placeholder={t("personality.instructionsPlaceholder")}
          onChange={(e) => set({ instructions: e.target.value })}
        />
        <div className="mt-1 flex items-center justify-between gap-3">
          <Link to="/settings/ai/knowledge" className="text-[12px] text-fg-3 underline decoration-line-strong underline-offset-2 hover:text-fg">
            {t("personality.knowledgeLink")}
          </Link>
          <Counter n={value.instructions.length} max={LIMITS.instructions} />
        </div>
      </Block>

      <Block label={t("personality.topics")} hint={t("personality.topicsHint")}>
        <ChipEditor
          values={value.blocked_topics}
          onChange={(blocked_topics) => set({ blocked_topics })}
          max={LIMITS.topics}
          placeholder={t("personality.topicsPlaceholder")}
          aria-label={t("personality.topics")}
          normalize={(v) => v.trim().replace(/\s+/g, " ")}
          validate={(v) => {
            const p = topicProblem(v);
            return p ? t(`personality.topic_${p}`) : null;
          }}
          dot="bg-tone-red-solid"
          empty={t("personality.topicsEmpty")}
        />
      </Block>
    </div>
  );
}

function Block({ label, hint, children, htmlFor }: { label: string; hint?: string; children: ReactNode; htmlFor?: string }) {
  return (
    <div>
      <div className="mb-2">
        {htmlFor ? (
          <label htmlFor={htmlFor} className="text-[12.5px] font-medium text-fg">
            {label}
          </label>
        ) : (
          <div className="text-[12.5px] font-medium text-fg">{label}</div>
        )}
        {hint && <div className="mt-0.5 text-[12px] leading-[18px] text-fg-3">{hint}</div>}
      </div>
      {children}
    </div>
  );
}

function TextField({
  id,
  label,
  hint,
  value,
  onChange,
  max,
  placeholder,
  area,
  rows = 3,
  required,
  className,
}: {
  id: string;
  label: string;
  hint?: string;
  value: string;
  onChange: (v: string) => void;
  max: number;
  placeholder?: string;
  area?: boolean;
  rows?: number;
  required?: boolean;
  className?: string;
}) {
  const { t } = useTranslation("bots");
  const p = textProblem(value, max, area);
  const err = p ? t(`personality.text_${p}`, { max }) : required && !value.trim() ? t("personality.required") : null;
  const common = { id, value, invalid: !!err, placeholder, "aria-describedby": err ? `${id}-err` : undefined };
  return (
    <div className={className}>
      <Block label={label} hint={hint} htmlFor={id}>
        {area ? <Textarea {...common} rows={rows} onChange={(e) => onChange(e.target.value)} /> : <TextInput {...common} onChange={(e) => onChange(e.target.value)} />}
        <div className="mt-1 flex items-start justify-between gap-3">
          {err ? (
            <span id={`${id}-err`} role="alert" className="text-xs text-danger-fg">
              {err}
            </span>
          ) : (
            <span />
          )}
          <Counter n={value.trim().length} max={max} />
        </div>
      </Block>
    </div>
  );
}

function ToneCards({ value, onChange, custom }: { value: BotTone; onChange: (v: BotTone) => void; custom: string }) {
  const { t } = useTranslation("bots");
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const step = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const to = (i + step + TONES.length) % TONES.length;
    onChange(TONES[to]);
    refs.current[to]?.focus();
  };
  return (
    <div role="radiogroup" aria-label={t("personality.tone")} className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
      {TONES.map((v, i) => {
        const on = v === value;
        const sample = v === "custom" ? custom.trim() || t("tone.custom_sample") : t(`tone.${v}_sample`);
        return (
          <button
            key={v}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="radio"
            aria-checked={on}
            tabIndex={on ? 0 : -1}
            onClick={() => onChange(v)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={cn(
              "group relative flex flex-col gap-2 rounded-lg border bg-surface p-3 text-left outline-none transition-[border-color,box-shadow,background-color] duration-150 focus-visible:shadow-[0_0_0_3px_var(--focus)] motion-reduce:transition-none",
              on ? "border-fg shadow-[inset_0_0_0_1px_var(--fg)]" : "border-line hover:border-line-strong hover:bg-sunken/50",
            )}
          >
            <div className="flex items-center justify-between gap-2">
              <span className="text-[12.5px] font-medium text-fg">{t(`tone.${v}`)}</span>
              {on && (
                <span className="flex size-4 items-center justify-center rounded-full bg-fg text-surface">
                  <Check className="size-2.5" strokeWidth={3} />
                </span>
              )}
            </div>
            <span className="text-[11.5px] leading-[16px] text-fg-3">{t(`tone.${v}_hint`)}</span>
            <span className={cn("mt-auto self-start rounded-xl rounded-tl-sm px-2.5 py-1.5 text-[11.5px] leading-[16px]", tone[AI_TONE].soft, "text-fg-2")}>{sample}</span>
          </button>
        );
      })}
    </div>
  );
}
