// The products an import reads (contracts/imports.md "Credentials"): what each asks for,
// where its people find it, and its mark.

import { FileSpreadsheet } from "lucide-react";
import type { ImportSource, ImportStatus } from "@/lib/api/types";
import type { ItemState } from "@/components/settings2/bits";
import { cn } from "@/lib/cn";
import { tone, type Tone } from "@/lib/tones";

export const SOURCES: ImportSource[] = ["zendesk", "helpscout", "freshdesk", "intercom", "csv"];

export const sourceName: Record<ImportSource, string> = {
  zendesk: "Zendesk",
  helpscout: "Help Scout",
  freshdesk: "Freshdesk",
  intercom: "Intercom",
  csv: "CSV file",
};

export type CredentialField = {
  key: string;
  label: string;
  /** Where to find the value, in the source's own words. */
  hint: string;
  secret?: boolean;
  optional?: boolean;
  /** Shown after the input, for an account address: ".zendesk.com". */
  suffix?: string;
  placeholder?: string;
  /** The source's help page that shows the value. */
  help?: string;
  /** A choice instead of text: value and label. */
  choices?: [string, string][];
};

export const credentialFields: Record<Exclude<ImportSource, "csv">, CredentialField[]> = {
  zendesk: [
    { key: "subdomain", label: "Zendesk address", hint: "The part before .zendesk.com in your Zendesk links.", suffix: ".zendesk.com", placeholder: "acme" },
    { key: "email", label: "Admin email", hint: "The email you sign in to Zendesk with. It must be an admin.", placeholder: "you@company.com" },
    {
      key: "token",
      label: "API token",
      hint: "In Admin Center, open Apps and integrations, then Zendesk API. Turn on token access and add a token.",
      secret: true,
      help: "https://support.zendesk.com/hc/en-us/articles/4408889192858",
    },
  ],
  helpscout: [
    {
      key: "app_id",
      label: "App ID",
      hint: "In Help Scout, open your profile, then My apps, and create an app. Any redirect URL works.",
      help: "https://developer.helpscout.com/mailbox-api/overview/authentication/#client-credentials-flow",
    },
    { key: "app_secret", label: "App secret", hint: "Shown next to the App ID.", secret: true },
    {
      key: "docs_key",
      label: "Docs API key",
      hint: "To bring your Docs articles too. In your profile, open Authentication, then API keys.",
      secret: true,
      optional: true,
      help: "https://developer.helpscout.com/docs-api/",
    },
  ],
  freshdesk: [
    { key: "domain", label: "Freshdesk address", hint: "The part before .freshdesk.com in your Freshdesk links.", suffix: ".freshdesk.com", placeholder: "acme" },
    {
      key: "api_key",
      label: "API key",
      hint: "Click your picture in Freshdesk, open Profile settings and choose View API key. Use an admin's key.",
      secret: true,
      help: "https://support.freshdesk.com/support/solutions/articles/215517",
    },
  ],
  intercom: [
    {
      key: "region",
      label: "Where your workspace is hosted",
      hint: "Most workspaces are in the US. Your Intercom address says if it is in Europe or Australia.",
      choices: [
        ["us", "United States"],
        ["eu", "Europe"],
        ["au", "Australia"],
      ],
    },
    {
      key: "token",
      label: "Access token",
      hint: "In the Developer Hub, open your app, then Configure and Authentication. A private app in your workspace is enough.",
      secret: true,
      help: "https://developers.intercom.com/docs/build-an-integration/learn-more/authentication",
    },
  ],
};

export const defaultCredentials = (src: ImportSource): Record<string, string> => (src === "intercom" ? { region: "us" } : {});

const markTone: Record<ImportSource, Tone> = { zendesk: "green", helpscout: "blue", freshdesk: "teal", intercom: "indigo", csv: "gray" };

/** A source's initial in its tone, or a sheet for a CSV file. No borrowed logos. */
export function SourceMark({ source, size = "md", className }: { source: ImportSource; size?: "sm" | "md" | "lg"; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "flex shrink-0 select-none items-center justify-center rounded-md font-semibold tracking-[-0.02em]",
        size === "sm" ? "size-5 text-[10.5px] [&_svg]:size-3" : size === "lg" ? "size-10 rounded-lg text-[17px] [&_svg]:size-5" : "size-8 text-[13.5px] [&_svg]:size-4",
        tone[markTone[source]].chip,
        className,
      )}
    >
      {source === "csv" ? <FileSpreadsheet /> : sourceName[source][0]}
    </span>
  );
}

export const statusState: Record<ImportStatus, ItemState> = {
  running: "active",
  paused: "paused",
  completed: "active",
  failed: "failing",
  canceled: "disabled",
  undoing: "pending",
  undone: "disabled",
};

/** Whether an import holds the workspace's one import slot. */
export const isActive = (s: ImportStatus) => s === "running" || s === "paused" || s === "undoing";

export const DOCS = "https://docs.opendesk.sh/guides/importing/";

/** Where an imported ticket came from (its custom.import), as "Zendesk #1234" with a link back. */
export function importedFrom(custom: Record<string, unknown> | undefined): { label: string; url: string | null } | null {
  const x = custom?.import as { source?: string; ref?: string; url?: string } | undefined;
  if (!x || typeof x !== "object" || !x.source) return null;
  const name = SOURCES.includes(x.source as ImportSource) ? sourceName[x.source as ImportSource] : x.source;
  const label = [name, typeof x.ref === "string" ? x.ref : ""].filter(Boolean).join(" ");
  const url = typeof x.url === "string" && x.url.startsWith("https://") ? x.url : null;
  return { label, url };
}
