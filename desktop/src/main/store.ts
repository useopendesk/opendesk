// A small JSON file in userData: the chosen server, window bounds per instance,
// the last theme and the update channel. Nothing secret lives here; the session stays in the
// browser cookie jar.

import { app } from "electron";
import { readFileSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";

export type Instance = {
  /** The web app origin, e.g. https://app.opendesk.sh. */
  appOrigin: string;
  /** The API origin from the instance's config.json (often the app origin). */
  apiOrigin: string;
};

export type Bounds = { x?: number; y?: number; width: number; height: number; maximized?: boolean };
export type ThemePref = "light" | "dark" | "system";
export type UpdateChannel = "stable" | "beta";

type Data = {
  instance?: Instance;
  bounds?: Record<string, Bounds>;
  theme?: ThemePref;
  updateChannel?: UpdateChannel;
};

let data: Data | null = null;

function file() {
  return join(app.getPath("userData"), "settings.json");
}

function load(): Data {
  if (data) return data;
  try {
    const raw = JSON.parse(readFileSync(file(), "utf8")) as unknown;
    data = raw && typeof raw === "object" ? (raw as Data) : {};
  } catch {
    data = {};
  }
  return data;
}

function save() {
  const tmp = `${file()}.tmp`;
  try {
    writeFileSync(tmp, JSON.stringify(load(), null, 2), { mode: 0o600 });
    renameSync(tmp, file());
  } catch (e) {
    console.error("settings: could not save", (e as Error).message);
  }
}

function validInstance(v: unknown): v is Instance {
  if (!v || typeof v !== "object") return false;
  const i = v as Instance;
  try {
    return new URL(i.appOrigin).origin === i.appOrigin && new URL(i.apiOrigin).origin === i.apiOrigin;
  } catch {
    return false;
  }
}

export const store = {
  instance(): Instance | undefined {
    const i = load().instance;
    return validInstance(i) ? i : undefined;
  },
  setInstance(i: Instance) {
    load().instance = i;
    save();
  },
  bounds(origin: string): Bounds | undefined {
    const b = load().bounds?.[origin];
    if (!b || typeof b.width !== "number" || typeof b.height !== "number") return undefined;
    return b;
  },
  setBounds(origin: string, b: Bounds) {
    const d = load();
    d.bounds = { ...d.bounds, [origin]: b };
    save();
  },
  theme(): ThemePref {
    const t = load().theme;
    return t === "dark" || t === "system" ? t : "light";
  },
  setTheme(t: ThemePref) {
    if (load().theme === t) return;
    load().theme = t;
    save();
  },
  /** Beta when chosen, or by default when this build is itself a prerelease. */
  updateChannel(): UpdateChannel {
    const c = load().updateChannel;
    if (c === "stable" || c === "beta") return c;
    return app.getVersion().includes("-") ? "beta" : "stable";
  },
  setUpdateChannel(c: UpdateChannel) {
    if (load().updateChannel === c) return;
    load().updateChannel = c;
    save();
  },
};
