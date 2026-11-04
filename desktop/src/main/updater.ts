// Auto-update from GitHub Releases (electron-updater, generic provider on the
// rolling desktop-latest release), checked at launch and every six hours.
// Installed builds only. The state is mirrored to the page
// (`window.opendeskDesktop.update`, contracts/native.md), which shows the
// "Update ready" banner.
//
// Always the newest version, in one step: an update is downloaded straight from
// the running version, however many releases came out since. While one waits for
// a restart the checks go on, and a newer one downloads behind it and takes its
// place once installable. Restarting looks once more first, so it installs the
// newest release out at that moment.
//
// Channels: stable reads latest*.yml, beta reads beta*.yml. A stable release
// also writes the beta files, so beta users get every stable version newer than
// their beta. Downgrades are never allowed: someone who leaves the beta keeps
// their beta version until a stable version passes it.

import { app, dialog, autoUpdater as squirrel, Notification } from "electron";
import { autoUpdater, type ProgressInfo, type UpdateInfo } from "electron-updater";
import { devArg, isDev, RELEASE_NOTES_URL, UPDATE_INTERVAL_MS } from "./env";
import { store, type UpdateChannel } from "./store";
import { newerVersion } from "./versions";

export type UpdateState =
  | { status: "idle" }
  | { status: "checking" }
  | { status: "available"; version: string }
  | { status: "downloading"; version: string; percent: number }
  | { status: "ready"; version: string; notes_url: string }
  | { status: "error" };

type Ready = Extract<UpdateState, { status: "ready" }>;

/** How long Restart waits on the last look for a newer version before installing what it has. */
const LAST_LOOK_MS = 8_000;

let state: UpdateState = { status: "idle" };
let started = false;
let onStateChange: ((s: UpdateState, statusChanged: boolean) => void) | null = null;
/** The downloaded version a restart installs; it stays while a newer one downloads. */
let ready: Ready | null = null;
/** The version downloading now. */
let fetching: string | null = null;
/** Restart was asked for: install as soon as the newest version is ready. */
let installing = false;

function set(next: UpdateState) {
  const statusChanged = next.status !== state.status;
  state = next;
  onStateChange?.(state, statusChanged);
}

/** Progress shows while nothing is ready, or once Restart waits for it. */
function quiet() {
  return !!ready && !installing;
}

export function updateState(): UpdateState {
  return state;
}

export function updateReady() {
  return state.status === "ready";
}

export function updateChannel(): UpdateChannel {
  return store.updateChannel();
}

function applyChannel(c: UpdateChannel) {
  autoUpdater.channel = c === "beta" ? "beta" : "latest";
  autoUpdater.allowPrerelease = c === "beta";
  // Setting the channel turns downgrades on; a version is only ever replaced by a newer one.
  autoUpdater.allowDowngrade = false;
}

/** Choose the channel and look for an update on it. */
export function setUpdateChannel(c: UpdateChannel) {
  if (c === store.updateChannel()) return;
  store.setUpdateChannel(c);
  if (!started) return void onStateChange?.(state, false);
  applyChannel(c);
  // A finished download stays ready; anything else starts over on the new channel.
  if (!ready) set({ status: "idle" });
  else onStateChange?.(state, false);
  checkInBackground();
}

/** Look for an update without telling anyone unless one is found. */
export function checkInBackground() {
  if (!started || state.status === "checking" || fetching) return;
  void autoUpdater.checkForUpdates().catch(() => {});
}

/**
 * On macOS a download is installable once Squirrel.Mac has taken it from
 * electron-updater and staged it; until then a restart would install the
 * version staged before. Elsewhere the downloaded installer is all it takes.
 */
function staged(): Promise<void> {
  if (process.platform !== "darwin") return Promise.resolve();
  return new Promise((resolve, reject) => {
    const done = () => {
      squirrel.off("error", fail);
      resolve();
    };
    const fail = (e: Error) => {
      squirrel.off("update-downloaded", done);
      reject(e);
    };
    squirrel.once("update-downloaded", done);
    squirrel.once("error", fail);
  });
}

let installedAt = 0;

function install() {
  // The last look and an error can both get here; quit once.
  if (!ready || Date.now() - installedAt < 10_000) return;
  installedAt = Date.now();
  // Silent on Windows (no installer window) and the app opens again after.
  autoUpdater.quitAndInstall(true, true);
  // If the app could not quit, Restart works again.
  setTimeout(() => (installing = false), 10_000).unref();
}

export function startUpdater(onChange: (s: UpdateState, statusChanged: boolean) => void) {
  onStateChange = onChange;
  if (started) return;
  if (!app.isPackaged) {
    demoState();
    return;
  }
  started = true;
  // Downloads start here, so a version no newer than the one ready is not fetched again.
  autoUpdater.autoDownload = false;
  autoUpdater.autoInstallOnAppQuit = true;
  autoUpdater.logger = null;
  applyChannel(store.updateChannel());

  autoUpdater.on("checking-for-update", () => {
    if (!ready) set({ status: "checking" });
  });
  autoUpdater.on("update-not-available", () => {
    if (!ready) set({ status: "idle" });
  });
  autoUpdater.on("update-available", (info: UpdateInfo) => {
    if (fetching || (ready && !newerVersion(info.version, ready.version))) return;
    fetching = info.version;
    // Once Restart waits on it, its progress shows straight away so the banner stays up.
    if (installing) set({ status: "downloading", version: info.version, percent: 0 });
    else if (!ready) set({ status: "available", version: info.version });
    void autoUpdater.downloadUpdate().catch(() => {});
  });
  autoUpdater.on("download-progress", (p: ProgressInfo) => {
    if (!fetching || quiet()) return;
    const percent = Math.max(0, Math.min(100, Math.floor(p.percent ?? 0)));
    if (state.status === "downloading" && state.percent === percent) return;
    set({ status: "downloading", version: fetching, percent });
  });
  autoUpdater.on("update-downloaded", (info: UpdateInfo) => {
    staged().then(
      () => {
        fetching = null;
        const next: Ready = { status: "ready", version: info.version, notes_url: RELEASE_NOTES_URL(info.version) };
        const fresh = next.version !== ready?.version;
        ready = next;
        set(next);
        if (installing) return install();
        if (!fresh || !Notification.isSupported()) return;
        const n = new Notification({ title: "Update ready", body: `opendesk ${info.version} installs when you restart.`, silent: true });
        n.on("click", () => restartToUpdate());
        n.show();
      },
      () => {}, // the "error" event has it
    );
  });
  autoUpdater.on("error", (e) => {
    console.warn("update check failed:", e?.message ?? String(e));
    // A download that failed half way is retried at the next check.
    fetching = null;
    if (ready) {
      // The version that is ready still installs; Restart does not wait on a newer one that failed.
      set(ready);
      if (installing) install();
    } else set({ status: "error" });
  });

  checkInBackground();
  setInterval(checkInBackground, UPDATE_INTERVAL_MS).unref();
}

/**
 * Install the newest version: one more look first, and a newer release than the
 * one ready downloads (with its progress shown) and installs instead.
 */
export function restartToUpdate() {
  if (!ready || installing) return;
  installing = true;
  if (fetching) {
    set({ status: "downloading", version: fetching, percent: 0 });
    return; // installs when it is ready
  }
  const look = autoUpdater.checkForUpdates().catch(() => null);
  const timeout = new Promise<null>((resolve) => setTimeout(() => resolve(null), LAST_LOOK_MS));
  void Promise.race([look, timeout]).then(() => {
    if (!fetching) install();
  });
}

/** From the menu: check now and say what happened. */
export async function checkNow(parent: Electron.BrowserWindow | null) {
  const show = (message: string, detail?: string) =>
    parent && !parent.isDestroyed() ? dialog.showMessageBox(parent, { message, detail, buttons: ["OK"] }) : dialog.showMessageBox({ message, detail, buttons: ["OK"] });
  if (!app.isPackaged) {
    await show("Updates are checked in installed builds", "Development builds don't update themselves.");
    return;
  }
  if (ready) {
    const r = await dialog.showMessageBox({ message: `opendesk ${ready.version} is ready`, detail: "Restart opendesk to install it.", buttons: ["Restart now", "Later"], defaultId: 0, cancelId: 1 });
    if (r.response === 0) restartToUpdate();
    return;
  }
  if (fetching) {
    await show(`opendesk ${fetching} is downloading`, "You'll be asked to restart when it's ready.");
    return;
  }
  try {
    const r = await autoUpdater.checkForUpdates();
    if (r?.isUpdateAvailable) await show(`opendesk ${r.updateInfo.version} is downloading`, "You'll be asked to restart when it's ready.");
    else
      await show(
        "opendesk is up to date",
        `Version ${app.getVersion()}${store.updateChannel() === "beta" ? ", on beta updates" : ""}`,
      );
  } catch {
    await show("Couldn't check for updates", "Check your connection and try again.");
  }
}

/**
 * Development builds: `--update=downloading|ready` shows that state to the page,
 * so the web app's update banner can be seen without a release.
 */
function demoState() {
  if (!isDev) return;
  const demo = devArg("update");
  if (demo === "ready") set({ status: "ready", version: "9.9.9", notes_url: RELEASE_NOTES_URL("9.9.9") });
  else if (demo === "downloading") set({ status: "downloading", version: "9.9.9", percent: 42 });
}
