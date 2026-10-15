// Labels, label groups and custom fields for the Margin demo workspace.
// Values come from a hash of the record, not the shared random stream, so the
// rest of the seed stays as it was.

import type { CustomField, FieldEntity, FieldOption, FieldValues, LabelColor, LabelScope, TagGroup } from "@/lib/api/types";
import type { WsData } from "./db";
import { applyLabels, makeTag, seedDefaultLabels, syncTagRefs } from "./labels";
import { DAY, ago, iso, NOW, uuid } from "./util";

/** Seed content names labels in kebab case; these are the labels they mean. */
const ALIASES: Record<string, string> = {
  bug: "Bug",
  "feature-request": "Feature request",
  billing: "Billing",
  question: "Question",
  onboarding: "Onboarding",
  api: "API",
  sso: "SSO",
  mobile: "Mobile",
  export: "Export",
  refund: "Refund",
  "urgent-customer": "Escalated",
};

export function labelName(raw: string): string {
  return ALIASES[raw] ?? raw;
}

/** A stable small number from a string. */
function h(s: string, mod: number): number {
  let x = 2166136261;
  for (let i = 0; i < s.length; i++) x = Math.imul(x ^ s.charCodeAt(i), 16777619) >>> 0;
  return x % mod;
}

function group(ws: WsData, name: string, color: LabelColor | null, exclusive: boolean): TagGroup {
  const at = ago(300 * DAY);
  const g: TagGroup = { id: uuid(), name, color, exclusive, position: ws.tagGroups.length, tag_count: 0, created_at: at, updated_at: at };
  ws.tagGroups.push(g);
  return g;
}

export function seedMarginLabels(ws: WsData) {
  seedDefaultLabels(ws);
  const at = ago(300 * DAY);
  for (const t of ws.tags) t.created_at = t.updated_at = at;
  const type = ws.tagGroups[0];
  type.created_at = type.updated_at = at;
  const vip = ws.tags.find((t) => t.name === "VIP")!;
  const feedback = ws.tags.find((t) => t.name === "Feedback")!;

  const area = group(ws, "Product area", null, false);
  const areas: [string, LabelColor, string][] = [
    ["SSO", "indigo", "SAML, Okta and Azure AD sign-in"],
    ["API", "cyan", "REST API, webhooks and tokens"],
    ["Sync", "teal", "Sync between devices and the web app"],
    ["Export", "lime", "PDF, Markdown and backup exports"],
    ["Mobile", "pink", "iOS and Android apps"],
    ["Onboarding", "yellow", "Workspace setup and first steps"],
  ];
  areas.forEach(([name, color, description], i) => makeTag(ws, name, { color, description, group_id: area.id, position: i, applies_to: ["tickets"], created_at: at, updated_at: at }));

  const stage = group(ws, "Account health", "amber", true);
  const stages: [string, LabelColor, string][] = [
    ["Healthy", "green", "Active and growing"],
    ["Watch", "amber", "Usage dropping or open escalations"],
    ["Churn risk", "red", "Talking about leaving or downgrading"],
  ];
  stages.forEach(([name, color, description], i) => makeTag(ws, name, { color, description, group_id: stage.id, position: i, applies_to: ["contacts", "companies"], created_at: at, updated_at: at }));

  makeTag(ws, "Refund", { color: "rose", description: "Money goes back to the customer", position: 2, applies_to: ["tickets"], created_at: at, updated_at: at });
  makeTag(ws, "Escalated", { color: "red", description: "Handed to engineering or a lead", position: 3, applies_to: ["tickets"], created_at: at, updated_at: at });
  makeTag(ws, "Beta tester", { color: "purple", description: "Joined the early access program", position: 4, applies_to: ["contacts"] as LabelScope[], created_at: at, updated_at: at });
  makeTag(ws, "Partner", { color: "indigo", description: "Resells or integrates Margin", position: 5, applies_to: ["companies"] as LabelScope[], created_at: at, updated_at: at });
  makeTag(ws, "Legacy plan", { color: "gray", description: "Still on the 2023 pricing", position: 6, archived: true, created_at: at, updated_at: at });
  vip.position = 0;
  feedback.position = 1;
}

/** Tickets without a Type get Question or Problem, so every row shows one. */
export function seedTicketTypes(ws: WsData) {
  for (const t of ws.tickets) {
    if (t.spam) continue;
    const typed = t.tags.some((x) => x.group_id === ws.tagGroups[0].id);
    if (!typed) t.tags = applyLabels(ws, "tickets", t.tags, { add: [h(t.subject, 3) === 0 ? "Problem" : "Question"] }, { create: false, lenient: true }).next;
  }
}

function field(ws: WsData, entity: FieldEntity, key: string, name: string, type: CustomField["type"], extra: Partial<CustomField> = {}) {
  const at = ago(250 * DAY);
  const f: CustomField = {
    id: uuid(),
    entity,
    key,
    name,
    type,
    options: [],
    required_on_solve: false,
    description: "",
    position: ws.customFields.filter((x) => x.entity === entity).length,
    archived: false,
    created_at: at,
    updated_at: at,
    ...extra,
  };
  ws.customFields.push(f);
  return f;
}

const opts = (...xs: [string, string, LabelColor][]): FieldOption[] => xs.map(([id, label, color]) => ({ id, label, color }));
const date = (daysFromNow: number) => iso(NOW + daysFromNow * DAY).slice(0, 10);

export function seedMarginFields(ws: WsData, users: { maya: string; jonas: string; lea: string; priya: string }) {
  // Tickets.
  const product = field(ws, "ticket", "product_area", "Product area", "select", {
    description: "Where in the product the issue lives",
    options: opts(["editor", "Editor", "blue"], ["sync", "Sync", "teal"], ["search", "Search", "cyan"], ["export", "Export", "lime"], ["mobile", "Mobile apps", "pink"], ["integrations", "Integrations", "indigo"], ["account", "Account", "gray"]),
  });
  field(ws, "ticket", "resolution", "Resolution", "select", {
    description: "How the ticket ended; needed before solving",
    required_on_solve: true,
    options: opts(["fixed", "Fixed", "green"], ["answered", "Answered", "blue"], ["workaround", "Workaround", "amber"], ["wont_fix", "Won't fix", "gray"], ["duplicate", "Duplicate", "gray"]),
  });
  field(ws, "ticket", "platforms", "Platforms", "multi_select", {
    options: opts(["macos", "macOS", "gray"], ["windows", "Windows", "blue"], ["ios", "iOS", "gray"], ["android", "Android", "green"], ["web", "Web", "violet"]),
  });
  field(ws, "ticket", "app_version", "App version", "text", { description: "From Settings, About" });
  field(ws, "ticket", "affected_users", "Affected users", "number");
  field(ws, "ticket", "regression", "Regression", "checkbox", { description: "Worked in an earlier version" });
  field(ws, "ticket", "issue_link", "Issue link", "url", { description: "Linear or GitHub issue" });
  field(ws, "ticket", "follow_up_on", "Follow up on", "date");
  field(ws, "ticket", "escalation_owner", "Escalation owner", "user");
  field(ws, "ticket", "refund_amount", "Refund amount (EUR)", "number", { archived: true, description: "Moved to the billing system" });

  // Contacts.
  field(ws, "contact", "role", "Role", "select", {
    options: opts(["admin", "Workspace admin", "indigo"], ["end_user", "End user", "gray"], ["billing", "Billing contact", "green"], ["developer", "Developer", "cyan"]),
  });
  field(ws, "contact", "nps", "Last NPS", "number", { description: "0 to 10, from the quarterly survey" });
  field(ws, "contact", "timezone", "Time zone", "text");
  field(ws, "contact", "newsletter", "Newsletter", "checkbox");

  // Companies.
  field(ws, "company", "plan", "Plan", "select", {
    options: opts(["startup", "Startup", "gray"], ["team", "Team", "blue"], ["business", "Business", "violet"], ["enterprise", "Enterprise", "amber"]),
  });
  field(ws, "company", "arr", "ARR (EUR)", "number");
  field(ws, "company", "renewal_date", "Renewal date", "date");
  field(ws, "company", "account_manager", "Account manager", "user");
  field(ws, "company", "website", "Website", "url");

  // Ticket values.
  const areaFor: Record<string, string> = { SSO: "account", API: "integrations", Sync: "sync", Export: "export", Mobile: "mobile", Onboarding: "editor", Billing: "account" };
  const platforms = ["macos", "windows", "ios", "android", "web"];
  for (const t of ws.tickets) {
    if (t.spam) continue;
    const v: FieldValues = {};
    const hit = t.tags.map((x) => areaFor[x.name]).find(Boolean);
    const area = hit ?? (h(t.id, 3) === 0 ? product.options[h(t.subject, product.options.length)].id : undefined);
    if (area) v.product_area = area;
    const isBug = t.tags.some((x) => x.name === "Bug" || x.name === "Problem");
    if (isBug || h(t.subject, 2) === 0) v.platforms = platforms.filter((_, i) => h(`${t.id}:${i}`, 3) === 0).slice(0, 3);
    if (isBug) {
      v.app_version = `4.${h(t.subject, 9) + 2}.${h(t.id, 5)}`;
      if (h(t.id, 2) === 0) v.regression = true;
      if (h(t.subject, 3) === 0) v.issue_link = `https://linear.app/margin/issue/FN-${1200 + h(t.id, 700)}`;
    }
    if (t.priority === "urgent" || t.priority === "high") v.affected_users = [1, 3, 12, 40, 180][h(t.id, 5)];
    if (t.status === "solved" || t.status === "closed") v.resolution = isBug ? (h(t.id, 3) === 0 ? "workaround" : "fixed") : h(t.id, 4) === 0 ? "duplicate" : "answered";
    if (t.status === "pending" || t.status === "on_hold") v.follow_up_on = date(1 + h(t.id, 6));
    if (t.tags.some((x) => x.name === "Escalated" || x.name === "SSO")) v.escalation_owner = users.jonas;
    for (const k of Object.keys(v)) if (Array.isArray(v[k]) && (v[k] as string[]).length === 0) delete v[k];
    t.fields = v;
  }

  // Contact values and labels.
  const zones = ["Europe/Berlin", "Europe/London", "America/New_York", "Europe/Paris", "Asia/Singapore"];
  ws.contacts.forEach((c, i) => {
    const v: FieldValues = {};
    if (c.company) v.role = (["admin", "end_user", "end_user", "billing", "developer"] as const)[h(c.id, 5)];
    if (h(c.email ?? c.name, 3) !== 0) v.nps = h(c.name, 11);
    v.timezone = zones[h(c.name, zones.length)];
    if (h(c.id, 2) === 0) v.newsletter = true;
    c.fields = v;
    const names: string[] = [];
    if (i === 0 || h(c.id, 9) === 0) names.push("VIP");
    if (h(c.name, 7) === 0) names.push("Beta tester");
    if (typeof v.nps === "number" && v.nps <= 4) names.push("Churn risk");
    else if (typeof v.nps === "number" && v.nps >= 9) names.push("Healthy");
    if (names.length) c.tags = applyLabels(ws, "contacts", [], { add: names }, { create: false, lenient: true }).next;
  });

  // Company values and labels.
  const plans = ["enterprise", "business", "team", "startup"];
  const managers = [users.lea, users.maya, users.priya];
  ws.companies.forEach((co, i) => {
    const plan = plans[i % 4];
    co.fields = {
      plan,
      arr: { enterprise: 84000, business: 24000, team: 6000, startup: 1800 }[plan]! + h(co.name, 20) * 600,
      renewal_date: date(10 + h(co.name, 300)),
      website: `https://${co.domain}`,
      ...(plan === "enterprise" || plan === "business" ? { account_manager: managers[h(co.id, 3)] } : {}),
    };
    const names = [["Healthy", "Watch", "Churn risk", "Healthy"][h(co.name, 4)]];
    if (h(co.name, 4) === 1) names.push("Partner");
    if (plan === "enterprise") names.push("VIP");
    co.tags = applyLabels(ws, "companies", [], { add: names }, { create: false, lenient: true }).next;
  });
  syncTagRefs(ws);
}
