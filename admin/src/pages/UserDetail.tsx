import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Ban, ChevronDown, LogOut, Mail, RotateCcw, ScrollText, ShieldOff, Trash2 } from "lucide-react";
import { useState } from "react";
import { ActionButton, ActionMenuItem } from "@/components/ActionButton";
import { IdChip, None } from "@/components/bits";
import { ErrorState } from "@/components/ErrorState";
import { BackLink, Banner, DetailSkeleton, KV, PageBody, PageColumn, PageHeader, Section } from "@/components/layout";
import { AdminRolePill, FlagPill, SuspendedPill } from "@/components/pills";
import { ReasonDialog } from "@/components/ReasonDialog";
import { StepUpDialog } from "@/components/StepUpDialog";
import { Table, TD, TH, THead, TR } from "@/components/table";
import { Time } from "@/components/Time";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { EmptyState } from "@/components/ui/empty";
import { Field, TextInput } from "@/components/ui/input";
import { Menu, MenuContent, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { toast } from "@/components/ui/toast";
import { useDocumentTitle } from "@/app/RootLayout";
import { isApiError } from "@/lib/api/client";
import { api } from "@/lib/api/endpoints";
import type { AdminSession, AdminUser } from "@/lib/api/types";
import { humanize, number, plural } from "@/lib/format";
import { q, qk } from "@/lib/queries";
import { useMe } from "@/lib/session";

export default function UserDetail() {
  const { id } = useParams({ from: "/shell/users/$id" });
  const user = useQuery(q.user(id));
  useDocumentTitle(user.data?.email);

  return (
    <>
      <PageHeader title={<BackLink to="/users">Users</BackLink>} />
      <PageBody>
        {user.isPending ? (
          <DetailSkeleton />
        ) : user.isError ? (
          <ErrorState
            error={user.error}
            onRetry={() => void user.refetch()}
            action={
              <Button variant="ghost" asChild>
                <Link to="/users">All users</Link>
              </Button>
            }
          />
        ) : (
          <UserView u={user.data} />
        )}
      </PageBody>
    </>
  );
}

const CLIENT: Record<AdminSession["client"], string> = { web: "Browser", desktop: "Desktop app", ios: "iPhone app" };

/** "Chrome on macOS" from a user agent, or the raw string cut short. */
function deviceOf(ua: string) {
  const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : /opendesk/i.test(ua) ? "opendesk" : "";
  const os = /iPhone|iPad/.test(ua) ? "iOS" : /Mac OS X|Macintosh/.test(ua) ? "macOS" : /Windows/.test(ua) ? "Windows" : /Android/.test(ua) ? "Android" : /Linux/.test(ua) ? "Linux" : "";
  if (browser && os) return `${browser} on ${os}`;
  return browser || os || ua.slice(0, 60) || "Unknown device";
}

function UserView({ u }: { u: AdminUser }) {
  const me = useMe();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const confirm = useConfirm();
  const [dialog, setDialog] = useState<null | "suspend" | "mfa" | "delete">(null);
  const [confirmEmail, setConfirmEmail] = useState("");
  const self = u.id === me.id;
  const suspendBlocked = self ? "You can't suspend yourself" : u.admin_role ? "Revoke their admin role first" : null;

  const apply = (next: AdminUser) => {
    qc.setQueryData(qk.user(u.id), next);
    void qc.invalidateQueries({ queryKey: qk.users });
  };
  const reload = () => qc.invalidateQueries({ queryKey: qk.user(u.id) });

  const resend = async () => {
    try {
      await api.users.resendVerification(u.id);
      toast.success(`Verification email sent to ${u.email}`);
    } catch (e) {
      if (isApiError(e, "already_verified")) {
        toast.info("This email is already verified");
        void reload();
      } else toast.error(e);
    }
  };

  const restore = () =>
    confirm.show(
      <>
        <span className="font-medium text-fg">{u.email}</span> can sign in again.
      </>,
      async () => {
        apply(await api.users.unsuspend(u.id));
        toast.success("User restored");
      },
      { title: "Restore this user?", confirmLabel: "Restore" },
    );

  const signOutAll = () =>
    confirm.show(
      <>
        Every session of <span className="font-medium text-fg">{u.email}</span> ends on every device, in the app and the panel. They can sign in again.
      </>,
      async () => {
        await api.users.revokeSessions(u.id);
        await reload();
        toast.success("Signed out everywhere");
      },
      { title: "Sign out everywhere?", confirmLabel: "Sign out everywhere", danger: true },
    );

  const signOutOne = async (s: AdminSession) => {
    try {
      await api.users.revokeSessions(u.id, s.id);
      qc.setQueryData<AdminUser>(qk.user(u.id), (prev) => (prev ? { ...prev, sessions: prev.sessions.filter((x) => x.id !== s.id) } : prev));
      toast.success("Session signed out");
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <PageColumn>
      <div className="flex flex-wrap items-start gap-4">
        <Avatar name={u.name || u.email} src={u.avatar_url} size="xl" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate text-[18px] font-semibold tracking-[-0.02em]">{u.name || u.email}</h1>
            {u.admin_role && <AdminRolePill role={u.admin_role} />}
            {u.suspended_at && <SuspendedPill />}
            {self && <Badge tone="outline">You</Badge>}
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            {u.name && <span className="text-[12.5px] text-fg-2">{u.email}</span>}
            <IdChip label="id" value={u.id} />
            <span className="text-[11.5px] text-fg-3">
              Joined <Time iso={u.created_at} />, last seen <Time iso={u.last_seen_at} />
            </span>
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          <Button variant="ghost" asChild icon={<ScrollText />}>
            <Link to="/audit" search={{ target: u.id }}>
              Audit log
            </Link>
          </Button>
          <Menu>
            <MenuTrigger asChild>
              <Button variant="secondary" iconRight={<ChevronDown />}>
                Actions
              </Button>
            </MenuTrigger>
            <MenuContent className="w-[260px]">
              {!u.email_verified && (
                <ActionMenuItem perm="users.sessions" icon={<Mail />} onSelect={() => void resend()}>
                  Resend verification email
                </ActionMenuItem>
              )}
              <ActionMenuItem perm="users.sessions" icon={<LogOut />} blocked={u.sessions.length ? null : "No live sessions"} onSelect={signOutAll}>
                Sign out everywhere
              </ActionMenuItem>
              {u.suspended_at ? (
                <ActionMenuItem perm="users.suspend" icon={<RotateCcw />} onSelect={restore}>
                  Restore user
                </ActionMenuItem>
              ) : (
                <ActionMenuItem perm="users.suspend" icon={<Ban />} blocked={suspendBlocked} onSelect={() => setDialog("suspend")}>
                  Suspend user
                </ActionMenuItem>
              )}
              <MenuSeparator />
              <ActionMenuItem
                perm="users.mfa"
                icon={<ShieldOff />}
                blocked={!u.totp_enabled ? "Two-factor authentication is off" : self ? "Turn yours off in the app" : null}
                onSelect={() => setDialog("mfa")}
              >
                Turn off two-factor authentication
              </ActionMenuItem>
              <ActionMenuItem perm="users.delete" icon={<Trash2 />} danger blocked={self ? "Delete your own account in the app" : null} onSelect={() => setDialog("delete")}>
                Delete user
              </ActionMenuItem>
            </MenuContent>
          </Menu>
        </div>
      </div>

      {u.suspended_at && (
        <Banner title={<>Suspended <Time iso={u.suspended_at} /></>}>
          {u.suspended_reason ? <span className="whitespace-pre-wrap">{u.suspended_reason}</span> : "No reason was given."} They can't sign in; their workspaces keep working for everyone else.
        </Banner>
      )}

      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_340px]">
        <div className="flex min-w-0 flex-col gap-5">
          <Section title="Workspaces" count={number(u.memberships.length)} flush>
            {u.memberships.length ? (
              <Table>
                <THead>
                  <TH>Workspace</TH>
                  <TH>Role</TH>
                  <TH>Joined</TH>
                </THead>
                <tbody>
                  {u.memberships.map((m) => (
                    <TR key={m.workspace.id}>
                      <TD>
                        <Link to="/workspaces/$id" params={{ id: m.workspace.id }} className="group flex min-w-0 items-center gap-2.5">
                          <Avatar name={m.workspace.name} square size="sm" />
                          <span className="min-w-0">
                            <span className="block truncate font-medium text-fg group-hover:underline">{m.workspace.name}</span>
                            <span className="mono block truncate text-[11px] text-fg-4">{m.workspace.slug}</span>
                          </span>
                        </Link>
                      </TD>
                      <TD>
                        <Badge tone={m.role === "owner" ? "indigo" : m.role === "admin" ? "blue" : "neutral"}>{humanize(m.role)}</Badge>
                      </TD>
                      <TD className="text-fg-3">
                        <Time iso={m.created_at} />
                      </TD>
                    </TR>
                  ))}
                </tbody>
              </Table>
            ) : (
              <EmptyState compact art="workspaces" title="No workspaces" description="This person isn't a member of any workspace." />
            )}
          </Section>

          <Section
            title="Live sessions"
            count={number(u.sessions.length)}
            flush
            actions={
              u.sessions.length > 0 && (
                <ActionButton perm="users.sessions" size="xs" variant="danger" onClick={signOutAll}>
                  Sign out everywhere
                </ActionButton>
              )
            }
          >
            {u.sessions.length ? (
              <Table>
                <THead>
                  <TH>Device</TH>
                  <TH>IP</TH>
                  <TH>Signed in</TH>
                  <TH>Last used</TH>
                  <TH />
                </THead>
                <tbody>
                  {u.sessions.map((s) => (
                    <TR key={s.id}>
                      <TD>
                        <span className="block font-medium text-fg" title={s.user_agent}>
                          {deviceOf(s.user_agent)}
                        </span>
                        <span className="flex items-center gap-1.5 text-[11.5px] text-fg-3">
                          {CLIENT[s.client] ?? s.client}
                          {s.mfa_verified && <Badge tone="green">2FA</Badge>}
                        </span>
                      </TD>
                      <TD mono className="text-[11.5px] text-fg-2">
                        {s.ip || "–"}
                      </TD>
                      <TD className="text-fg-3">
                        <Time iso={s.created_at} />
                      </TD>
                      <TD className="text-fg-2">
                        <Time iso={s.last_used_at} />
                      </TD>
                      <TD align="right">
                        <ActionButton perm="users.sessions" size="xs" variant="ghost" onClick={() => void signOutOne(s)}>
                          Sign out
                        </ActionButton>
                      </TD>
                    </TR>
                  ))}
                </tbody>
              </Table>
            ) : (
              <EmptyState compact art="users" title="No live sessions" description="They're signed out everywhere." />
            )}
          </Section>
        </div>

        <div className="flex min-w-0 flex-col gap-5">
          <Section title="Account">
            <KV
              items={[
                ["Email", <FlagPill on={u.email_verified} yes="Verified" no="Unverified" warnOff />],
                ["Two-factor", <FlagPill on={u.totp_enabled} yes="On" no="Off" />],
                ["Password", <FlagPill on={u.has_password} yes="Set" no="None" />],
                ["Passkeys", u.passkeys ? <span className="mono tabular-nums">{number(u.passkeys)}</span> : <None />],
                ["Admin role", u.admin_role ? <AdminRolePill role={u.admin_role} /> : <None />],
              ]}
            />
          </Section>
          <Section title="Sign-in identities" count={number(u.identities.length)}>
            {u.identities.length ? (
              <ul className="flex flex-col gap-2">
                {u.identities.map((i) => (
                  <li key={`${i.provider}:${i.email}`} className="flex items-center gap-2 text-[12.5px]">
                    <Badge tone="neutral">{humanize(i.provider)}</Badge>
                    <span className="min-w-0 flex-1 truncate text-fg-2">{i.email}</span>
                    <Time iso={i.created_at} className="text-[11.5px] text-fg-4" />
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-[12.5px] text-fg-3">Email and password only.</p>
            )}
          </Section>
        </div>
      </div>

      <ReasonDialog
        open={dialog === "suspend"}
        onOpenChange={(o) => setDialog(o ? "suspend" : null)}
        title={`Suspend ${u.email}?`}
        description="Every session ends, in the app and the panel, and they can't sign in until restored. Their workspaces keep working for everyone else."
        confirmLabel="Suspend user"
        onConfirm={async (reason) => {
          apply(await api.users.suspend(u.id, reason));
          toast.success("User suspended");
        }}
      />

      <StepUpDialog
        open={dialog === "mfa"}
        onOpenChange={(o) => setDialog(o ? "mfa" : null)}
        title="Turn off two-factor authentication?"
        description={
          <>
            The authenticator and recovery codes of {u.email} are removed (passkeys stay), every session ends, and they are emailed that an administrator did it. Do this only after
            confirming who is asking.
          </>
        }
        confirmLabel="Turn off 2FA"
        danger
        onConfirm={async (code) => {
          apply(await api.users.disableMfa(u.id, code));
          toast.success("Two-factor authentication turned off");
        }}
      />

      <StepUpDialog
        open={dialog === "delete"}
        onOpenChange={(o) => {
          setDialog(o ? "delete" : null);
          if (!o) setConfirmEmail("");
        }}
        title={`Delete ${u.email}?`}
        description={`The account and its data are removed for good, the same as the user's own Delete account. ${u.memberships.length ? `They leave ${plural(u.memberships.length, "workspace")}.` : ""} This can't be undone.`}
        confirmLabel="Delete user"
        danger
        ready={confirmEmail.trim().toLowerCase() === u.email.toLowerCase()}
        explain={(e) =>
          isApiError(e, "sole_owner") ? "They own a workspace that has other members. Transfer it or delete that workspace first." : undefined
        }
        onConfirm={async (code) => {
          await api.users.remove(u.id, code, confirmEmail.trim());
          qc.removeQueries({ queryKey: qk.user(u.id) });
          void qc.invalidateQueries({ queryKey: qk.users });
          void qc.invalidateQueries({ queryKey: qk.summary });
          toast.success(`${u.email} was deleted`);
          void navigate({ to: "/users" });
        }}
      >
        <Field label={<>Type <span className="mono font-medium">{u.email}</span> to confirm</>}>
          {(p) => <TextInput {...p} autoFocus autoComplete="off" spellCheck={false} value={confirmEmail} onChange={(e) => setConfirmEmail(e.target.value)} />}
        </Field>
      </StepUpDialog>
    </PageColumn>
  );
}
