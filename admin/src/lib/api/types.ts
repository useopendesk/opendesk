// The admin API's shapes, as contracts/admin.md defines them (snake_case JSON).

export type Role = "owner" | "admin" | "agent" | "viewer";

export type AdminRole = "viewer" | "support" | "super";
export type AdminPermission =
  | "read"
  | "users.sessions"
  | "users.suspend"
  | "users.mfa"
  | "users.delete"
  | "workspaces.suspend"
  | "workspaces.limits"
  | "workspaces.delete"
  | "jobs.manage"
  | "admins.manage";

export type AdminInstance = {
  version: string;
  commit: string;
  billing_enabled: boolean;
  ai_enabled: boolean;
  app_url: string;
  api_url: string;
  self_hosted: boolean;
};
export type AdminMe = { id: string; email: string; name: string; role: AdminRole; permissions: AdminPermission[]; instance: AdminInstance };
export type AdminAuthResult = { access_token: string; expires_in: number; admin: AdminMe };

export type Page<T> = { data: T[]; next_cursor: string | null };

export type AdminUserRow = {
  id: string;
  email: string;
  name: string;
  avatar_url: string | null;
  email_verified: boolean;
  totp_enabled: boolean;
  has_password: boolean;
  workspace_count: number;
  admin_role: AdminRole | null;
  suspended_at: string | null;
  last_seen_at: string | null;
  created_at: string;
};
export type AdminSession = {
  id: string;
  client: "web" | "desktop" | "ios";
  ip: string;
  user_agent: string;
  mfa_verified: boolean;
  created_at: string;
  last_used_at: string;
};
export type AdminUser = AdminUserRow & {
  suspended_reason: string;
  identities: { provider: string; email: string; created_at: string }[];
  passkeys: number;
  memberships: { workspace: AdminWorkspaceRef; role: Role; created_at: string }[];
  sessions: AdminSession[];
};

export type Plan = "free" | "pro" | "self_hosted";
export type AdminWorkspaceRef = { id: string; name: string; slug: string };
export type AdminWorkspaceRow = AdminWorkspaceRef & {
  avatar_url: string | null;
  members: number;
  plan: Plan;
  status: string;
  suspended_at: string | null;
  last_activity_at: string | null;
  created_at: string;
};
export type AdminLimits = { max_mailboxes: number | null; max_members: number | null; monthly_ai_usd: number | null };
export type AdminMailbox = {
  id: string;
  name: string;
  email: string;
  inbound_kind: string;
  outbound_kind: string;
  status: "active" | "error" | "disabled";
  last_error: string;
  last_synced_at: string | null;
  created_at: string;
};
export type AdminSubscription = {
  plan: "free" | "pro";
  status: string;
  interval: "month" | "year";
  seats: number;
  billed_seats: number;
  current_period_end: string | null;
  cancel_at_period_end: boolean;
  stripe_customer_id: string | null;
  stripe_subscription_id: string | null;
};
export type AdminWorkspaceAi = {
  remaining_usd: number;
  monthly_usd: number;
  included_used_usd: number;
  purchased_usd: number;
  unlimited: boolean;
  period_start: string;
  period_end: string;
  month: {
    charged_usd: number;
    provider_cost_usd: number;
    calls: number;
    by_feature: { feature: string; charged_usd: number; provider_cost_usd: number; calls: number }[];
  };
};

/** The workspace's own audit log entry (contracts/api.md). */
export type UserRef = { id: string; name: string; email: string; avatar_url: string | null };
export type AuditLog = {
  id: string;
  actor: { type: "user" | "api_key" | "system"; user: UserRef | null; api_key_id: string | null };
  action: string;
  entity_type: string;
  entity_id: string | null;
  data: Record<string, unknown>;
  ip: string;
  created_at: string;
};

export type AdminWorkspace = AdminWorkspaceRow & {
  suspended_reason: string;
  members_list: { user: { id: string; email: string; name: string }; role: Role; created_at: string }[];
  pending_invitations: number;
  counts: { tickets: number; open_tickets: number; contacts: number; kb_articles: number; api_keys: number; webhooks: number; brands: number };
  mailboxes: AdminMailbox[];
  /** 0 = unlimited, after overrides. */
  entitlements: { max_mailboxes: number; max_members: number; monthly_ai_usd: number; unlimited_ai: boolean };
  limits: AdminLimits;
  subscription: AdminSubscription | null;
  ai: AdminWorkspaceAi;
  recent_audit: AuditLog[];
};

export type AdminSummary = {
  users: { total: number; new_7d: number; new_30d: number; active_7d: number };
  workspaces: { total: number; new_7d: number; new_30d: number; active_7d: number; suspended: number };
  billing: { enabled: boolean; paid_workspaces: number; billed_seats: number; past_due: number };
  ai: { month_charged_usd: number; month_provider_cost_usd: number; month_calls: number };
  daily: { date: string; signups: number; workspaces: number; ai_charged_usd: number }[];
};

export type HealthStatus = "ok" | "warn" | "fail";
export type AdminHealth = {
  version: string;
  commit: string;
  role: string;
  started_at: string;
  database: { ok: boolean; latency_ms: number; schema_version: number; dirty: boolean; pool: { total: number; idle: number; in_use: number; max: number } };
  nats: { ok: boolean; embedded: boolean };
  relay: { unpublished: number; oldest_unpublished_s: number | null };
  jobs: {
    available: number;
    running: number;
    retryable: number;
    scheduled: number;
    discarded_24h: number;
    oldest_available_s: number | null;
    by_queue: { queue: string; available: number; running: number; retryable: number }[];
  };
  mail: { driver: string; inbound_failed_24h: number; inbound_pending: number; mailboxes_in_error: number };
  webhooks: { failing_24h: number };
  config: {
    billing: boolean;
    ai_provider: string;
    decisions_provider: string;
    embeddings: string;
    storage: string;
    signup_enabled: boolean;
    mailbox_imap: boolean;
    mailbox_system_send: boolean;
    push: string;
    help_domain: string;
  };
  checks: { name: string; status: HealthStatus; detail: string }[];
};

export const JOB_STATES = ["retryable", "discarded", "available", "running", "scheduled", "pending", "cancelled", "completed"] as const;
export type JobState = (typeof JOB_STATES)[number];
export type AdminJob = {
  id: number;
  kind: string;
  queue: string;
  state: JobState;
  attempt: number;
  max_attempts: number;
  args: Record<string, unknown>;
  last_error: string | null;
  errors: number;
  created_at: string;
  scheduled_at: string;
  attempted_at: string | null;
  finalized_at: string | null;
};
export type AdminJobStats = {
  states: Record<string, number>;
  kinds: { kind: string; available: number; running: number; retryable: number; discarded_24h: number }[];
};

export type InstanceAdmin = {
  user: { id: string; email: string; name: string };
  role: AdminRole;
  granted_by: { id: string; email: string } | null;
  created_at: string;
};

export type AdminAuditTarget = "user" | "workspace" | "admin" | "job" | "instance";
export type AdminAuditLog = {
  id: string;
  actor: { type: "admin" | "cli"; id: string | null; email: string };
  action: string;
  target_type: AdminAuditTarget;
  target_id: string | null;
  data: Record<string, unknown>;
  ip: string;
  created_at: string;
};

export const AUDIT_ACTIONS = [
  "login",
  "login.failed",
  "logout",
  "user.sessions_revoked",
  "user.verification_sent",
  "user.suspended",
  "user.unsuspended",
  "user.mfa_disabled",
  "user.deleted",
  "workspace.suspended",
  "workspace.unsuspended",
  "workspace.limits_changed",
  "workspace.ai_granted",
  "workspace.deleted",
  "job.retried",
  "job.cancelled",
  "admin.granted",
  "admin.role_changed",
  "admin.revoked",
] as const;
