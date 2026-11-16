// WhatsApp (contracts/whatsapp.md): connect a number through a business solution provider so
// customers' messages land on their tickets and the team answers from the ticket.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, KeyRound, MessageSquareText, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AppMark } from "@/components/appmark";
import { Row, Section, SettingsPage } from "@/components/layout";
import { type ItemState, ListSkeleton, StatePill } from "@/components/settings2/bits";
import { SettingsEmpty } from "@/components/settings2/SettingsEmpty";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { CopyField } from "@/components/ui/copy-button";
import { Dialog } from "@/components/ui/dialog";
import { Field, TextInput } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { toast } from "@/components/ui/toast";
import { errorMessage } from "@/lib/api/client";
import { api } from "@/lib/api/endpoints";
import type { WhatsAppConnection, WhatsAppProvider } from "@/lib/api/types";
import { relShort } from "@/lib/format";
import { q, qk, removeFromList, upsertInList } from "@/lib/queries";
import { atLeast, useRole } from "@/lib/session";

const REUSE = [0, 1, 3, 7, 14, 30] as const;
const NO_BRAND = "none";
const FIELDS = ["account_sid", "auth_token"] as const;
const knownField = (k: string): k is (typeof FIELDS)[number] => (FIELDS as readonly string[]).includes(k);

/** Nothing has come in at the webhook yet: the admin still has to set it in the provider. */
const waiting = (c: WhatsAppConnection) => c.health_code === "webhook_missing";

function connectionState(c: WhatsAppConnection): { state: ItemState; key: "healthy" | "action_required" | "error" | "unknown" | "waiting" } {
  if (c.status === "error" || c.health_status === "error") return { state: "failing", key: "error" };
  if (c.health_status === "healthy") return { state: "active", key: "healthy" };
  if (waiting(c)) return { state: "pending", key: "waiting" };
  if (c.health_status === "action_required") return { state: "pending", key: "action_required" };
  return { state: "disabled", key: "unknown" };
}

export default function WhatsApp() {
  const { t } = useTranslation("settingsChannels");
  const admin = atLeast(useRole(), "admin");
  const qc = useQueryClient();
  const confirm = useConfirm();
  const { data: providers } = useQuery(q.whatsappProviders());
  const { data: connections, isLoading } = useQuery(q.whatsappConnections());
  const { data: brands } = useQuery(q.brands());
  const [connecting, setConnecting] = useState(false);
  const [rotating, setRotating] = useState<WhatsAppConnection | null>(null);
  const provider = providers?.[0];

  const test = useMutation({
    mutationFn: api.whatsapp.test,
    onSuccess: (row) => {
      upsertInList(qc, qk.whatsappConnections, row);
      if (row.health_status === "healthy") toast.success(t("whatsapp.health.tested"));
      else toast.info(t("whatsapp.health.testedAttention"));
    },
    onError: (e) => toast.error(e, t("whatsapp.health.testError")),
  });
  const update = useMutation({
    mutationFn: ({ id, ...b }: { id: string; reuse_days?: number; brand_id?: string; clear_brand?: boolean }) => api.whatsapp.update(id, b),
    onSuccess: (row) => upsertInList(qc, qk.whatsappConnections, row),
    onError: (e) => toast.error(e, t("whatsapp.saveError")),
  });
  const disconnect = useMutation({
    mutationFn: api.whatsapp.disconnect,
    onSuccess: (_, id) => {
      removeFromList(qc, qk.whatsappConnections, id);
      toast.success(t("whatsapp.disconnected"));
    },
    onError: (e) => toast.error(e, t("whatsapp.disconnectError")),
  });

  const connect = admin && provider && (
    <Button icon={<Plus />} onClick={() => setConnecting(true)}>
      {t("whatsapp.connect.button")}
    </Button>
  );
  return (
    <SettingsPage wide title={t("whatsapp.title")} description={t("whatsapp.description")}>
      <Section flush title={t("whatsapp.connections.title")} description={t("whatsapp.connections.description")} actions={!!connections?.length && connect}>
        {isLoading ? (
          <ListSkeleton rows={2} />
        ) : !connections?.length ? (
          <SettingsEmpty
            art="settings"
            title={t("whatsapp.empty.title")}
            description={t("whatsapp.empty.body")}
            action={connect}
            tips={[
              { label: t("whatsapp.empty.tipInbox"), icon: <MessageSquareText /> },
              { label: t("whatsapp.empty.tipWindow"), icon: <Clock /> },
            ]}
            className="min-h-[260px]"
          />
        ) : (
          <div className="divide-y divide-line-soft">
            {connections.map((c) => {
              const state = connectionState(c);
              const detail = c.health_code ? t(`whatsapp.healthCode.${c.health_code}`) : c.health_detail;
              return (
                <div key={c.id} className="px-4 py-3.5" data-testid="whatsapp-connection">
                  <div className="flex items-start gap-3">
                    <AppMark app={c.provider} name={provider?.name ?? c.provider} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-[13px] font-medium">{c.name}</span>
                        <span className="mono text-[12px] text-fg-3">{c.sender}</span>
                        <StatePill state={state.state}>{t(`whatsapp.health.${state.key}`)}</StatePill>
                      </div>
                      {detail && <p className={state.state === "failing" ? "mt-1 text-[12px] text-danger-fg" : "mt-1 text-[12px] text-fg-2"}>{detail}</p>}
                      <p className="mt-0.5 text-[12px] text-fg-3">{c.last_event_at ? t("whatsapp.lastEvent", { when: relShort(c.last_event_at) }) : t("whatsapp.noEventYet")}</p>
                      {admin && c.webhook_url && (
                        <div className="mt-2 max-w-[560px]">
                          <p className="mb-1 text-[11.5px] text-fg-4">{t("whatsapp.webhook", { name: provider?.name ?? c.provider })}</p>
                          <CopyField value={c.webhook_url} />
                          {waiting(c) && <TwilioSteps className="mt-2.5" />}
                        </div>
                      )}
                    </div>
                    {admin && (
                      <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                        <Button icon={<RefreshCw />} loading={test.isPending && test.variables === c.id} onClick={() => test.mutate(c.id)}>
                          {t("whatsapp.health.test")}
                        </Button>
                        <Button square icon={<KeyRound />} aria-label={t("whatsapp.rotate.label", { name: c.name })} onClick={() => setRotating(c)} />
                        <Button
                          variant="danger"
                          square
                          icon={<Trash2 />}
                          aria-label={t("whatsapp.disconnectLabel", { name: c.name })}
                          onClick={() =>
                            confirm.show(t("whatsapp.disconnectBody", { name: c.name }), () => disconnect.mutateAsync(c.id), {
                              title: t("whatsapp.disconnectTitle"),
                              confirmLabel: t("whatsapp.disconnect"),
                              danger: true,
                            })
                          }
                        />
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Section>

      {connections?.map((c) => (
        <Section key={c.id} title={t("whatsapp.placement.title", { name: c.name })} description={t("whatsapp.placement.description")}>
          <Row label={t("whatsapp.placement.brand")} description={t("whatsapp.placement.brandHint")}>
            <Select
              size="sm"
              className="w-[200px]"
              aria-label={t("whatsapp.placement.brand")}
              disabled={!admin}
              value={c.brand_id ?? NO_BRAND}
              onChange={(v) => update.mutate(v === NO_BRAND ? { id: c.id, clear_brand: true } : { id: c.id, brand_id: v })}
              options={[{ value: NO_BRAND, label: t("whatsapp.placement.defaultBrand") }, ...(brands ?? []).map((b) => ({ value: b.id, label: b.name }))]}
            />
          </Row>
          <Row label={t("whatsapp.placement.reuse")} description={c.reuse_days ? t("whatsapp.placement.reuseHint", { count: c.reuse_days }) : t("whatsapp.placement.reuseNever")}>
            <Select
              size="sm"
              className="w-[180px]"
              aria-label={t("whatsapp.placement.reuse")}
              disabled={!admin}
              value={String(c.reuse_days)}
              onChange={(v) => update.mutate({ id: c.id, reuse_days: Number(v) })}
              options={REUSE.map((n) => ({ value: String(n), label: n === 0 ? t("whatsapp.placement.never") : n === 1 ? t("whatsapp.placement.day") : t("whatsapp.placement.daysN", { n }) }))}
            />
          </Row>
          <Row label={t("whatsapp.placement.window")} description={t("whatsapp.placement.windowHint")} />
        </Section>
      ))}

      {connecting && provider && <ConnectDialog provider={provider} onClose={() => setConnecting(false)} />}
      {rotating && <ConnectDialog provider={provider} connection={rotating} onClose={() => setRotating(null)} />}
    </SettingsPage>
  );
}

/** Where to set the webhook in Twilio's console. */
function TwilioSteps({ className }: { className?: string }) {
  const { t } = useTranslation("settingsChannels");
  const steps = [t("whatsapp.twilio.step1"), t("whatsapp.twilio.step2"), t("whatsapp.twilio.step3")];
  return (
    <ol className={className} data-testid="webhook-steps">
      {steps.map((step, i) => (
        <li key={i} className="flex gap-2 py-0.5 text-[12px] leading-[18px] text-fg-2">
          <span className="mono w-4 shrink-0 text-right text-fg-4">{i + 1}.</span>
          <span>{step}</span>
        </li>
      ))}
    </ol>
  );
}

/** Connect a number, or replace a connection's credentials. */
function ConnectDialog({ provider, connection, onClose }: { provider?: WhatsAppProvider; connection?: WhatsAppConnection; onClose: () => void }) {
  const { t } = useTranslation("settingsChannels");
  const qc = useQueryClient();
  const { data: brands } = useQuery(q.brands());
  const [credentials, setCredentials] = useState<Record<string, string>>({});
  const [sender, setSender] = useState("");
  const [name, setName] = useState("");
  const [brand, setBrand] = useState(NO_BRAND);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<WhatsAppConnection | null>(null);
  const providerName = provider?.name ?? connection?.provider ?? "";
  const save = useMutation({
    mutationFn: () =>
      connection
        ? api.whatsapp.update(connection.id, { credentials })
        : api.whatsapp.connect({ provider: provider?.key ?? "", sender, name: name.trim() || undefined, brand_id: brand === NO_BRAND ? undefined : brand, credentials }),
    onSuccess: (row) => {
      upsertInList(qc, qk.whatsappConnections, row);
      toast.success(connection ? t("whatsapp.rotate.saved") : t("whatsapp.connect.connected", { name: providerName }));
      if (connection) onClose();
      else setCreated(row);
    },
    onError: (e) => setError(errorMessage(e, t("whatsapp.connect.error", { name: providerName }))),
  });
  const fields = provider?.fields ?? [];
  const valid = fields.length > 0 && fields.every((f) => credentials[f.name]?.trim()) && (!!connection || sender.trim().length > 0);
  if (created) {
    // The next step is the webhook, set in the provider's console.
    return (
      <Dialog
        open
        onOpenChange={(open) => !open && onClose()}
        title={t("whatsapp.hook.title")}
        description={t("whatsapp.hook.description", { name: providerName })}
        footer={
          <Button variant="primary" onClick={onClose}>
            {t("whatsapp.hook.done")}
          </Button>
        }
      >
        <div className="flex flex-col gap-3">
          {created.webhook_url && <CopyField value={created.webhook_url} />}
          <TwilioSteps />
        </div>
      </Dialog>
    );
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={connection ? t("whatsapp.rotate.title", { name: connection.name }) : t("whatsapp.connect.title", { name: providerName })}
      description={connection ? undefined : t("whatsapp.connect.setup")}
      footer={
        <>
          <Button onClick={onClose}>{t("whatsapp.connect.cancel")}</Button>
          <Button variant="primary" disabled={!valid} loading={save.isPending} onClick={() => save.mutate()}>
            {connection ? t("whatsapp.rotate.submit") : t("whatsapp.connect.submit")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {!connection && (
          <>
            <Field label={t("whatsapp.connect.sender")} hint={t("whatsapp.connect.senderHint")}>
              {(props) => <TextInput {...props} inputMode="tel" autoComplete="off" className="mono" placeholder="+14155550100" value={sender} onChange={(e) => setSender(e.target.value)} />}
            </Field>
            <Field label={t("whatsapp.connect.name")}>
              {(props) => <TextInput {...props} autoComplete="off" maxLength={80} placeholder={providerName} value={name} onChange={(e) => setName(e.target.value)} />}
            </Field>
            {!!brands?.length && brands.length > 1 && (
              <Field label={t("whatsapp.placement.brand")}>
                {(props) => (
                  <Select
                    {...props}
                    value={brand}
                    onChange={setBrand}
                    options={[{ value: NO_BRAND, label: t("whatsapp.placement.defaultBrand") }, ...brands.map((b) => ({ value: b.id, label: b.name }))]}
                  />
                )}
              </Field>
            )}
          </>
        )}
        {fields.map((f) => (
          <Field key={f.name} label={knownField(f.name) ? t(`whatsapp.field.${f.name}`) : f.label}>
            {(props) => (
              <TextInput
                {...props}
                type={f.secret ? "password" : "text"}
                autoComplete="off"
                spellCheck={false}
                className="mono"
                value={credentials[f.name] ?? ""}
                onChange={(e) => setCredentials((old) => ({ ...old, [f.name]: e.target.value }))}
              />
            )}
          </Field>
        ))}
        {error && <p className="text-[12px] text-danger-fg">{error}</p>}
      </div>
    </Dialog>
  );
}
