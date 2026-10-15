// The widget's topics (contracts/teams.md "Widget"): what the visitor picks
// before the first message, each routed to a team. Reorderable, at most 8.
// New topics carry a local "new_" id until the server gives them one.

import { ArrowRight, Plus, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { SortableList } from "@/components/settings2/Sortable";
import { TeamPick } from "@/components/settings2/teams";
import { Button } from "@/components/ui/button";
import { TextInput } from "@/components/ui/input";
import { Tooltip } from "@/components/ui/tooltip";
import type { WidgetTopic } from "@/lib/api/types";
import { hasMarkup, tidy } from "./fields";

export const MAX_TOPICS = 8;

export function topicProblem(t: WidgetTopic, all: WidgetTopic[]): "length" | "markup" | "duplicate" | null {
  const label = tidy(t.label);
  if (label.length < 2 || label.length > 40) return "length";
  if (hasMarkup(label)) return "markup";
  if (all.some((x) => x !== t && tidy(x.label).toLowerCase() === label.toLowerCase())) return "duplicate";
  return null;
}

/** What goes up: tidy labels, and no id on new topics. */
export const topicsForSave = (topics: WidgetTopic[]): WidgetTopic[] =>
  topics.map((t) => ({ id: t.id.startsWith("new_") ? "" : t.id, label: tidy(t.label), team_id: t.team_id }));

export function TopicsEditor({ value, onChange, disabled }: { value: WidgetTopic[]; onChange: (v: WidgetTopic[]) => void; disabled?: boolean }) {
  const { t } = useTranslation("settingsTeams");
  const full = value.length >= MAX_TOPICS;
  const patch = (id: string, p: Partial<WidgetTopic>) => onChange(value.map((x) => (x.id === id ? { ...x, ...p } : x)));
  const add = () => onChange([...value, { id: `new_${Date.now().toString(36)}`, label: "", team_id: null }]);

  return (
    <div className="flex flex-col gap-2">
      {value.length > 0 && (
        <div className="overflow-hidden rounded-md border border-line">
          <SortableList
            items={value}
            disabled={disabled}
            handleLabel={(x) => t("topics.reorder", { label: x.label || t("topics.untitled") })}
            onReorder={(ids) => onChange(ids.map((id) => value.find((x) => x.id === id)!).filter(Boolean))}
            renderRow={(x, handle, i) => {
              const problem = x.label ? topicProblem(x, value) : null;
              return (
                <div className="group relative flex flex-col gap-1 px-3 py-2 pl-6">
                  {handle}
                  <div className="flex items-center gap-2">
                    <span className="mono w-4 shrink-0 text-right text-[11px] tabular-nums text-fg-4">{i + 1}</span>
                    <TextInput
                      inputSize="sm"
                      className="min-w-0 flex-1"
                      aria-label={t("topics.label", { n: i + 1 })}
                      placeholder={t("topics.placeholder")}
                      value={x.label}
                      maxLength={50}
                      invalid={!!problem}
                      disabled={disabled}
                      autoFocus={!x.label && x.id.startsWith("new_")}
                      onChange={(e) => patch(x.id, { label: e.target.value })}
                    />
                    <ArrowRight className="size-3.5 shrink-0 text-fg-4" aria-label={t("topics.goesTo")} />
                    <TeamPick className="w-[150px]" value={x.team_id} noneLabel={t("topics.widgetTeam")} disabled={disabled} onChange={(team_id) => patch(x.id, { team_id })} />
                    {!disabled && (
                      <Tooltip content={t("topics.remove")}>
                        <Button size="sm" variant="ghost" square aria-label={t("topics.removeNamed", { label: x.label })} onClick={() => onChange(value.filter((y) => y.id !== x.id))}>
                          <X className="size-3.5" />
                        </Button>
                      </Tooltip>
                    )}
                  </div>
                  {problem && (
                    <p role="alert" className="pl-6 text-[11.5px] text-danger-fg">
                      {t(`topics.errors.${problem}`)}
                    </p>
                  )}
                </div>
              );
            }}
          />
        </div>
      )}
      <div className="flex items-center justify-between gap-3">
        <span className="text-[12px] text-fg-3">{value.length ? t("topics.count", { n: value.length, max: MAX_TOPICS }) : t("topics.none")}</span>
        {!disabled && (
          <Button size="sm" icon={<Plus className="size-3.5" />} disabled={full} onClick={add}>
            {t("topics.add")}
          </Button>
        )}
      </div>
    </div>
  );
}
