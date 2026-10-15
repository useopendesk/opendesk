// Custom fields in the automation builder: `field.<key>` (the ticket's field)
// and `contact.field.<key>` (the ticket contact's) conditions with the
// operators of the field's type, and the value editor shared with `set_field`.

import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { TextInput } from "@/components/ui/input";
import { Segmented } from "@/components/ui/tabs";
import type { PickerItem } from "@/components/ui/picker";
import type { CustomField, FieldCondition, FieldOp, FieldType } from "@/lib/api/types";
import { q } from "@/lib/queries";
import { cn } from "@/lib/cn";
import { isTone, tone as tones } from "@/lib/tones";
import { MultiPick, SinglePick, useMemberItems } from "./pickers";
import { typeIcon } from "./fields/model";
import { DateField } from "@/components/ui/date-picker";

export type FieldRef = `field.${string}` | `contact.field.${string}`;

export const isFieldRef = (f: string): f is FieldRef => f.startsWith("field.") || f.startsWith("contact.field.");

/** The entity and key a condition field names. */
export function parseFieldRef(f: FieldRef): { entity: "ticket" | "contact"; key: string } {
  return f.startsWith("contact.") ? { entity: "contact", key: f.slice("contact.field.".length) } : { entity: "ticket", key: f.slice("field.".length) };
}

/** Operators per field type (contracts/api.md, custom field filters). */
export function fieldOpsFor(type: FieldType | undefined): FieldOp[] {
  switch (type) {
    case "number":
    case "date":
      return ["eq", "ne", "gt", "gte", "lt", "lte", "set", "not_set"];
    case "checkbox":
      return ["eq", "ne", "set", "not_set"];
    case "select":
    case "user":
      return ["eq", "ne", "any", "none", "set", "not_set"];
    case "multi_select":
      return ["any", "all", "none", "eq", "ne", "set", "not_set"];
    default:
      return ["eq", "ne", "contains", "not_contains", "set", "not_set"];
  }
}

export const opTakesValue = (op: FieldOp) => op !== "set" && op !== "not_set";
const opTakesList = (op: FieldOp) => op === "any" || op === "all" || op === "none";

/** Every custom field, with the one a rule already names kept even if archived. */
export function useAutomationFields() {
  const { data } = useQuery(q.customFields());
  const all = data ?? [];
  const find = (ref: FieldRef): CustomField | undefined => {
    const { entity, key } = parseFieldRef(ref);
    return all.find((f) => f.entity === entity && f.key === key);
  };
  return { all, find };
}

/** Picker items for condition fields: ticket fields, then contact fields. */
export function useFieldConditionItems(current?: string): PickerItem[] {
  const { t } = useTranslation("settingsInbox");
  const { all } = useAutomationFields();
  return all
    .filter((f) => (f.entity === "ticket" || f.entity === "contact") && (!f.archived || current === refOf(f)))
    .sort((a, b) => (a.entity === b.entity ? a.position - b.position : a.entity === "ticket" ? -1 : 1))
    .map((f) => ({
      value: refOf(f),
      label: f.name,
      icon: <span className="flex text-fg-3">{typeIcon[f.type]}</span>,
      group: f.entity === "ticket" ? t("customFields.ticketGroup") : t("customFields.contactGroup"),
      keywords: [f.key],
    }));
}

export const refOf = (f: CustomField): FieldRef => (f.entity === "contact" ? `contact.field.${f.key}` : `field.${f.key}`);

export function useFieldOpLabel() {
  const { t } = useTranslation("settingsInbox");
  return (op: FieldOp, type?: FieldType) => {
    if ((type === "number" || type === "date") && (op === "gt" || op === "gte" || op === "lt" || op === "lte")) return t(`customFields.ops.${type}.${op}`);
    return t(`customFields.ops.${op}`);
  };
}

/** A condition on a custom field is complete when its value is set, or its operator needs none. */
export function fieldConditionComplete(c: FieldCondition) {
  return !opTakesValue(c.op) || !!c.value.trim();
}

const optionItems = (f: CustomField): PickerItem[] =>
  f.options.map((o) => ({
    value: o.id,
    label: o.label,
    icon: <span className={cn("!size-2 rounded-xs", tones[isTone(o.color) ? o.color : "gray"].solid)} />,
  }));

/**
 * The value editor for a field: options for selects, people for user fields,
 * yes / no for checkboxes, a date or number box, or text. A list operator
 * (any, all, none) takes several values, written comma separated.
 */
export function FieldValueInput({
  field,
  op = "eq",
  value,
  onChange,
  invalid,
  allowClear,
}: {
  field: CustomField | undefined;
  op?: FieldOp;
  value: string;
  onChange: (v: string) => void;
  invalid?: boolean;
  /** set_field: an empty value clears the field. */
  allowClear?: boolean;
}) {
  const { t } = useTranslation("settingsInbox");
  const members = useMemberItems();
  if (!field) return <span className="text-[12px] text-fg-4">{t("customFields.missing")}</span>;
  if (!opTakesValue(op)) return <span className="text-[12px] text-fg-3">{t("customFields.noValue")}</span>;
  const list = value ? value.split(",").filter(Boolean) : [];
  const multi = field.type === "multi_select" ? op !== "eq" && op !== "ne" : opTakesList(op);
  const multiValue = allowClear && field.type === "multi_select";

  switch (field.type) {
    case "select":
    case "multi_select":
      return multi || multiValue ? (
        <MultiPick items={optionItems(field)} values={list} onChange={(v) => onChange(v.join(","))} placeholder={t("customFields.chooseOptions")} className="w-full" />
      ) : (
        <SinglePick items={optionItems(field)} value={value} onChange={onChange} placeholder={t("customFields.chooseOption")} invalid={invalid} className="w-full" />
      );
    case "user":
      return multi ? (
        <MultiPick items={members} values={list} onChange={(v) => onChange(v.join(","))} placeholder={t("customFields.choosePeople")} className="w-full" />
      ) : (
        <SinglePick items={members} value={value} onChange={onChange} placeholder={t("customFields.choosePerson")} invalid={invalid} width={280} className="w-full" />
      );
    case "checkbox":
      return (
        <Segmented
          value={value === "true" ? "true" : value === "false" ? "false" : ("" as "true" | "false")}
          onChange={onChange}
          options={[
            { value: "true", label: t("customFields.yes") },
            { value: "false", label: t("customFields.no") },
          ]}
        />
      );
    case "date":
      return (
        <DateField
          size="sm"
          value={value}
          invalid={invalid}
          aria-label={t("customFields.valueFor", { name: field.name })}
          onChange={onChange}
          className="w-[170px]"
        />
      );
    case "number":
      return (
        <TextInput
          inputSize="sm"
          inputMode="decimal"
          value={value}
          invalid={invalid || (!!value && !Number.isFinite(Number(value)))}
          aria-label={t("customFields.valueFor", { name: field.name })}
          placeholder="0"
          onChange={(e) => onChange(e.target.value.replace(/[^0-9.,-]/g, "").replace(",", "."))}
          className="mono w-[140px]"
        />
      );
    default:
      return (
        <TextInput
          inputSize="sm"
          value={value}
          invalid={invalid}
          aria-label={t("customFields.valueFor", { name: field.name })}
          placeholder={allowClear ? t("customFields.clearHint") : field.type === "email" ? "@acme.com" : field.type === "url" ? "https://" : ""}
          onChange={(e) => onChange(e.target.value)}
          className={field.type === "email" || field.type === "url" ? "mono" : undefined}
        />
      );
  }
}
