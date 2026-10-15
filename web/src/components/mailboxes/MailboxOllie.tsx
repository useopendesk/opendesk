// The AI section of a mailbox (contracts/agent.md "Email"): the mailbox's bot answers
// emails to this mailbox, off by default, either as a suggested reply a teammate
// sends or by replying itself. Only offered while AI is on.

import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Row, Section } from "@/components/layout";
import { Switch } from "@/components/ui/switch";
import { Segmented } from "@/components/ui/tabs";
import { OllieAvatar, useAssistantName } from "@/components/tickets/ollie";
import { BotFace } from "@/components/bots/BotFace";
import { BotSelect, resolveBot } from "@/components/bots/BotSelect";
import type { MailboxAIMode } from "@/lib/api/types";
import { useAI } from "@/lib/ai";
import { formatUsd } from "@/lib/format";
import { q } from "@/lib/queries";
import { AI_TONE, tone } from "@/lib/tones";
import { cn } from "@/lib/cn";

export type MailboxOllieDraft = { ai_answers: boolean; ai_answers_mode: MailboxAIMode; ai_answers_outside_hours: boolean; bot_id: string | null };

export function MailboxOllieSection({ value, onChange }: { value: MailboxOllieDraft; onChange: (patch: Partial<MailboxOllieDraft>) => void }) {
  const { t } = useTranslation("settingsChannels");
  const { t: tb } = useTranslation("bots");
  const assistant = useAssistantName();
  const ai = useAI();
  // The section speaks as the bot this address answers as.
  const { data: bots } = useQuery({ ...q.bots(), enabled: ai.enabled });
  const bot = resolveBot(bots, value.bot_id);
  const name = bot?.name ?? assistant;
  const { data: hours } = useQuery({ ...q.businessHours(), enabled: ai.enabled });
  const cost = ai.cost("answer_email");
  const on = ai.enabled && value.ai_answers;
  const showHours = on && (hours?.enabled || value.ai_answers_outside_hours);

  return (
    <Section
      title={
        <span className="inline-flex items-center gap-2">
          {bot ? <BotFace look={bot.look} image={bot.image_url} size={16} /> : <OllieAvatar size={16} />}
          {name}
        </span>
      }
      description={t("ollie.description", { name })}
    >
      <Row
        label={t("ollie.toggle", { name })}
        description={
          ai.enabled ? (
            t("ollie.toggleHint")
          ) : (
            <>
              {t("ollie.aiOff")}{" "}
              <Link to="/settings/ai" className="font-medium text-fg-2 underline decoration-line-strong underline-offset-2 hover:text-fg">
                {t("ollie.aiOffLink")}
              </Link>
            </>
          )
        }
      >
        <Switch aria-label={t("ollie.toggle", { name })} checked={on} disabled={!ai.enabled} onCheckedChange={(v) => onChange({ ai_answers: v })} />
      </Row>
      {on && (
        <>
          <Row
            label={tb("picker.label")}
            description={
              <>
                {tb("picker.mailboxHint")}{" "}
                <Link to="/settings/ai/bots" className="text-fg-2 underline decoration-line-strong underline-offset-2 hover:text-fg">
                  {tb("picker.manage")}
                </Link>
              </>
            }
            htmlFor="mailbox-bot"
          >
            <BotSelect id="mailbox-bot" className="w-[240px]" value={value.bot_id} onChange={(bot_id) => onChange({ bot_id })} />
          </Row>
          <Row label={t("ollie.mode")} description={value.ai_answers_mode === "send" ? t("ollie.sendHint", { name }) : t("ollie.draftHint", { name })}>
            <Segmented
              size="xs"
              value={value.ai_answers_mode}
              onChange={(v: MailboxAIMode) => onChange({ ai_answers_mode: v })}
              options={[
                { value: "draft", label: t("ollie.draft") },
                { value: "send", label: t("ollie.send") },
              ]}
            />
          </Row>
          {showHours && (
            <Row label={t("ollie.outsideHours")} description={hours?.enabled ? t("ollie.outsideHoursHint") : t("ollie.hoursOff")}>
              <Switch
                aria-label={t("ollie.outsideHours")}
                checked={value.ai_answers_outside_hours}
                onCheckedChange={(v) => onChange({ ai_answers_outside_hours: v })}
              />
            </Row>
          )}
          <div className={cn("-mx-4 border-t px-4 py-3 text-[12.5px] leading-[18px] text-fg-2", tone[AI_TONE].soft, tone[AI_TONE].line)}>
            {cost != null ? t("ollie.cost", { cost: formatUsd(cost, { precise: true }) }) : null} {t("ollie.handoff", { name })}{" "}
            <Link to="/settings/ai/behaviour" className={cn("font-medium underline-offset-2 hover:underline", tone[AI_TONE].text)}>
              {t("ollie.behaviourLink")}
            </Link>
          </div>
        </>
      )}
    </Section>
  );
}
