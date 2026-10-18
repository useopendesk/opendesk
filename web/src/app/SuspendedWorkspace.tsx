// The page a member sees while an instance admin has their workspace suspended
// (contracts/admin.md): every workspace route answers 403 workspace_suspended, so
// the app shows why instead of the inbox, and offers their other workspaces.

import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty";
import { useSession } from "@/lib/session";
import { useAccountActions } from "./useAccounts";

export function SuspendedWorkspace() {
  const { t } = useTranslation("shell");
  const { switchTo, logOut } = useAccountActions();
  const uid = useSession((s) => s.user?.id);
  const current = useSession((s) => s.memberships.find((m) => m.workspace.id === s.workspaceId));
  const other = useSession((s) => s.memberships.find((m) => m.workspace.id !== s.workspaceId && !m.suspended));

  return (
    <div className="fixed inset-0 flex flex-col bg-surface text-fg">
      <EmptyState
        art="spam"
        title={t("suspended.title")}
        description={`${t("suspended.body", { name: current?.workspace.name ?? "" })} ${t("suspended.contact")}`}
        action={
          other && uid ? (
            <Button variant="primary" onClick={() => void switchTo(uid, other.workspace.id)}>
              {t("suspended.open", { name: other.workspace.name })}
            </Button>
          ) : undefined
        }
        secondary={<Button onClick={() => void logOut()}>{t("suspended.signOut")}</Button>}
      />
    </div>
  );
}
