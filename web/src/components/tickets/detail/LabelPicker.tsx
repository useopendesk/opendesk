// The ticket's labels: grouped, searchable, keyboard driven, with inline
// create when the workspace allows it. An exclusive group holds one label, so
// picking another from it swaps them (the server applies the same rule).

import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { q } from "@/lib/queries";
import type { LabelScope } from "@/lib/api/types";
import { atLeast, useRole } from "@/lib/session";
import { Picker, type PickerItem } from "@/components/ui/picker";
import { TagSwatch } from "../TagSwatch";
import { useDisplay, useLabelGroups, type LabelRef } from "../fields";

export function useLabelChoices(scope: LabelScope = "tickets") {
  const { t } = useTranslation("inbox");
  const { data: tags } = useQuery(q.tags());
  const { data: groups } = useLabelGroups();
  const labels = ((tags ?? []) as LabelRef[]).filter((x) => !x.archived && (!x.applies_to || x.applies_to.includes(scope)));
  const sortedGroups = [...(groups ?? [])].sort((a, b) => a.position - b.position);
  const items: PickerItem[] = [];
  for (const g of sortedGroups) {
    for (const x of labels.filter((l) => l.group_id === g.id))
      items.push({ value: x.name, label: x.name, icon: <TagSwatch name={x.name} color={x.color} />, group: g.exclusive ? t("labels.pickOne", { name: g.name }) : g.name });
  }
  const loose = labels.filter((l) => !l.group_id || !sortedGroups.some((g) => g.id === l.group_id));
  for (const x of loose) items.push({ value: x.name, label: x.name, icon: <TagSwatch name={x.name} color={x.color} />, group: sortedGroups.length ? t("labels.other") : undefined });

  /** Apply exclusive groups: a newly added label drops the others of its group. */
  const resolve = (prev: string[], next: string[]) => {
    const added = next.filter((n) => !prev.includes(n));
    let out = next;
    for (const name of added) {
      const l = labels.find((x) => x.name === name);
      const g = l?.group_id ? sortedGroups.find((x) => x.id === l.group_id) : undefined;
      if (g?.exclusive) out = out.filter((n) => n === name || labels.find((x) => x.name === n)?.group_id !== g.id);
    }
    return out;
  };
  return { items, resolve };
}

export function LabelPicker({
  values,
  onChange,
  trigger,
  side,
  scope,
}: {
  values: string[];
  onChange: (v: string[]) => void;
  trigger: ReactNode;
  side?: "top" | "bottom";
  /** Which records the labels apply to; tickets by default. */
  scope?: LabelScope;
}) {
  const { t } = useTranslation("inbox");
  const { items, resolve } = useLabelChoices(scope);
  const display = useDisplay();
  const role = useRole();
  const canCreate = display.labelCreation !== "admins" || atLeast(role, "admin");
  return (
    <Picker
      multi
      side={side}
      width={260}
      values={values}
      placeholder={canCreate ? t("labels.addOrCreate") : t("labels.add")}
      emptyText={canCreate ? t("labels.typeToCreate") : t("labels.none")}
      createLabel={t("props.createTag")}
      items={items}
      onChange={(v) => onChange(resolve(values, v))}
      onCreate={canCreate ? (name) => onChange([...values, name]) : undefined}
      trigger={trigger}
    />
  );
}
