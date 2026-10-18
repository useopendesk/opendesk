import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { UserPlus } from "lucide-react";
import { useState } from "react";
import { ActionButton } from "@/components/ActionButton";
import { ErrorState } from "@/components/ErrorState";
import { PageBody, PageHeader } from "@/components/layout";
import { AdminRolePill } from "@/components/pills";
import { StepUpDialog } from "@/components/StepUpDialog";
import { Table, TableSkeleton, TD, TH, THead, TR } from "@/components/table";
import { Time } from "@/components/Time";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { EmptyState, GhostList } from "@/components/ui/empty";
import { Field, TextInput } from "@/components/ui/input";
import { Segmented } from "@/components/ui/tabs";
import { toast } from "@/components/ui/toast";
import { isApiError } from "@/lib/api/client";
import { api } from "@/lib/api/endpoints";
import type { AdminRole, InstanceAdmin } from "@/lib/api/types";
import { q, qk } from "@/lib/queries";
import { ROLE_HINT, ROLE_LABEL, ROLES, roleRank } from "@/lib/permissions";
import { useMe } from "@/lib/session";

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function explainGrant(e: unknown): string | undefined {
  if (isApiError(e, "not_found") || isApiError(e, "user_not_found")) return "No account uses this email. They sign up in the app first.";
  if (isApiError(e, "admin_mfa_setup_required") || isApiError(e, "target_mfa_required"))
    return "They need two-factor authentication on in the app before they can be an admin.";
  if (isApiError(e, "already_admin")) return "They are already an admin. Change their role in the list instead.";
  if (isApiError(e, "account_suspended")) return "This account is suspended. Restore it before making them an admin.";
  return undefined;
}

function RolePicker({ value, onChange }: { value: AdminRole; onChange: (r: AdminRole) => void }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[12.5px] font-medium">Role</span>
      <Segmented value={value} onChange={onChange} options={[...ROLES].reverse().map((r) => ({ value: r, label: ROLE_LABEL[r] }))} />
      <span className="text-[12px] text-fg-3">{ROLE_HINT[value]}</span>
    </div>
  );
}

export default function Admins() {
  const me = useMe();
  const qc = useQueryClient();
  const admins = useQuery(q.admins());
  const [granting, setGranting] = useState(false);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<AdminRole>("support");
  const [editing, setEditing] = useState<{ a: InstanceAdmin; role: AdminRole } | null>(null);
  const [revoking, setRevoking] = useState<InstanceAdmin | null>(null);

  const list = [...(admins.data ?? [])].sort((a, b) => roleRank(b.role) - roleRank(a.role) || a.user.email.localeCompare(b.user.email));
  const supers = list.filter((a) => a.role === "super").length;
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: qk.admins });
    void qc.invalidateQueries({ queryKey: qk.users });
  };

  /** Why a row's admin can't be changed or revoked from here, or null. */
  const lockedReason = (a: InstanceAdmin, action: "change" | "revoke") => {
    if (a.user.id === me.id) return action === "change" ? "You can't change your own role" : "You can't revoke yourself";
    if (a.role === "super" && supers <= 1) return "The last super admin stays. The command line can change it.";
    return null;
  };

  return (
    <>
      <PageHeader
        title="Admins"
        subtitle="Who can open this panel"
        actions={
          <ActionButton perm="admins.manage" variant="primary" icon={<UserPlus />} onClick={() => setGranting(true)}>
            Add an admin
          </ActionButton>
        }
      />
      <PageBody>
        {admins.isPending ? (
          <TableSkeleton cols={5} rows={4} />
        ) : admins.isError ? (
          <ErrorState error={admins.error} onRetry={() => void admins.refetch()} />
        ) : !list.length ? (
          <EmptyState
            art="admins"
            ghost={<GhostList variant="people" />}
            title="No admins"
            description="The first admin is granted on the machine that runs the instance, with opendesk admin grant."
          />
        ) : (
          <Table>
            <THead>
              <TH>Admin</TH>
              <TH>Role</TH>
              <TH>Granted by</TH>
              <TH>Since</TH>
              <TH />
            </THead>
            <tbody>
              {list.map((a) => (
                <TR key={a.user.id}>
                  <TD>
                    <Link to="/users/$id" params={{ id: a.user.id }} className="group flex min-w-0 items-center gap-2.5">
                      <Avatar name={a.user.name || a.user.email} size="md" />
                      <span className="min-w-0">
                        <span className="flex items-center gap-1.5">
                          <span className="truncate font-medium text-fg group-hover:underline">{a.user.name || a.user.email}</span>
                          {a.user.id === me.id && <Badge tone="outline">You</Badge>}
                        </span>
                        {a.user.name && <span className="block truncate text-[11.5px] text-fg-3">{a.user.email}</span>}
                      </span>
                    </Link>
                  </TD>
                  <TD>
                    <AdminRolePill role={a.role} />
                  </TD>
                  <TD className="text-fg-2">{a.granted_by ? a.granted_by.email : <Badge tone="outline">Command line</Badge>}</TD>
                  <TD className="text-fg-3">
                    <Time iso={a.created_at} />
                  </TD>
                  <TD align="right">
                    <div className="flex items-center justify-end gap-1">
                      <ActionButton perm="admins.manage" size="xs" variant="secondary" blocked={lockedReason(a, "change")} onClick={() => setEditing({ a, role: a.role })}>
                        Change role
                      </ActionButton>
                      <ActionButton perm="admins.manage" size="xs" variant="danger" blocked={lockedReason(a, "revoke")} onClick={() => setRevoking(a)}>
                        Revoke
                      </ActionButton>
                    </div>
                  </TD>
                </TR>
              ))}
            </tbody>
          </Table>
        )}
      </PageBody>

      <StepUpDialog
        open={granting}
        onOpenChange={(o) => {
          setGranting(o);
          if (!o) {
            setEmail("");
            setRole("support");
          }
        }}
        title="Add an admin"
        description="They need an account with two-factor authentication on. They sign in here with their own email, password and code."
        confirmLabel="Add admin"
        ready={EMAIL.test(email.trim())}
        explain={explainGrant}
        onConfirm={async (code) => {
          const r = await api.admins.grant({ email: email.trim(), role, code });
          refresh();
          toast.success(`${r.user.email} is now a ${ROLE_LABEL[r.role].toLowerCase()} admin`);
        }}
      >
        <Field label="Email">{(p) => <TextInput {...p} type="email" autoFocus autoComplete="off" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" />}</Field>
        <RolePicker value={role} onChange={setRole} />
      </StepUpDialog>

      <StepUpDialog
        open={!!editing}
        onOpenChange={(o) => !o && setEditing(null)}
        title={editing ? `Change the role of ${editing.a.user.email}` : "Change role"}
        description="The new role applies at once, on their next request."
        confirmLabel="Change role"
        ready={!!editing && editing.role !== editing.a.role}
        onConfirm={async (code) => {
          if (!editing) return;
          await api.admins.setRole(editing.a.user.id, editing.role, code);
          refresh();
          toast.success(`Role changed to ${ROLE_LABEL[editing.role]}`);
        }}
      >
        {editing && <RolePicker value={editing.role} onChange={(r) => setEditing({ ...editing, role: r })} />}
      </StepUpDialog>

      <StepUpDialog
        open={!!revoking}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={revoking ? `Revoke ${revoking.user.email}?` : "Revoke admin"}
        description="They lose access to the panel at once and their panel sessions end. Their account and workspaces are not touched."
        confirmLabel="Revoke admin"
        danger
        onConfirm={async (code) => {
          if (!revoking) return;
          await api.admins.revoke(revoking.user.id, code);
          refresh();
          toast.success(`${revoking.user.email} is no longer an admin`);
        }}
      />
    </>
  );
}
