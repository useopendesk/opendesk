import { useSyncExternalStore } from "react";

// Alert sounds, synthesized with Web Audio so there is no asset to load. Browsers allow
// audio only after the person interacted with the page, so the context is created and
// resumed on the first pointer or key press; until then a sound is skipped silently.

let ctx: AudioContext | null = null;

function audio(): AudioContext | null {
  if (typeof window === "undefined" || !("AudioContext" in window)) return null;
  ctx ??= new AudioContext();
  return ctx;
}

if (typeof window !== "undefined") {
  const unlock = () => {
    const c = audio();
    if (c && c.state === "suspended") void c.resume();
  };
  window.addEventListener("pointerdown", unlock, { capture: true, passive: true });
  window.addEventListener("keydown", unlock, { capture: true });
}

function tone(c: AudioContext, freq: number, start: number, length: number, volume: number, type: OscillatorType = "sine") {
  const osc = c.createOscillator();
  const gain = c.createGain();
  osc.type = type;
  osc.frequency.setValueAtTime(freq, start);
  gain.gain.setValueAtTime(0, start);
  gain.gain.linearRampToValueAtTime(volume, start + 0.012);
  gain.gain.exponentialRampToValueAtTime(0.0001, start + length);
  osc.connect(gain).connect(c.destination);
  osc.start(start);
  osc.stop(start + length + 0.02);
}

export type AlertSound = "waiting" | "reminder" | "ticket" | "reply" | "assigned";

/**
 * Live chat, a visitor is on the page right now:
 *   waiting: a bright rising three-note chime, a visitor needs a person now.
 *   reminder: a softer two-note ping, they are still waiting.
 * Everyday work, quiet and low so they never sound like the chat alarm:
 *   ticket: a soft falling two-note drop, a new conversation arrived.
 *   reply: a single muted tap, a customer answered on your ticket.
 *   assigned: a light two-note lift, something was assigned to you or you were mentioned.
 */
export function playAlert(kind: AlertSound) {
  const c = audio();
  if (!c || c.state !== "running") return;
  const t = c.currentTime + 0.01;
  switch (kind) {
    case "waiting":
      tone(c, 880, t, 0.22, 0.22);
      tone(c, 1174.66, t + 0.11, 0.24, 0.22);
      tone(c, 1567.98, t + 0.22, 0.42, 0.2);
      return;
    case "reminder":
      tone(c, 1046.5, t, 0.2, 0.12);
      tone(c, 1318.51, t + 0.12, 0.32, 0.1);
      return;
    case "ticket":
      tone(c, 659.25, t, 0.16, 0.07, "triangle");
      tone(c, 493.88, t + 0.1, 0.3, 0.06, "triangle");
      return;
    case "reply":
      tone(c, 587.33, t, 0.18, 0.08, "triangle");
      return;
    case "assigned":
      tone(c, 698.46, t, 0.14, 0.07, "triangle");
      tone(c, 880, t + 0.09, 0.26, 0.06, "triangle");
      return;
  }
}

// ── Everyday sounds, each on its own switch, per browser ─────────────────────────────

/** The everyday sounds a person can turn on or off; live chat has its own switch below. */
export type SoundPref = "ticket" | "reply" | "assigned";

const PREFS_KEY = "od-sounds";
const prefListeners = new Set<() => void>();

function readPrefs(): Record<SoundPref, boolean> {
  const all = { ticket: true, reply: true, assigned: true };
  try {
    const raw = JSON.parse(localStorage.getItem(PREFS_KEY) ?? "{}") as Partial<Record<SoundPref, unknown>>;
    for (const k of Object.keys(all) as SoundPref[]) if (typeof raw[k] === "boolean") all[k] = raw[k] as boolean;
  } catch {
    /* storage unavailable or malformed: defaults */
  }
  return all;
}

let prefs = readPrefs();

export function setSoundPref(kind: SoundPref, on: boolean) {
  prefs = { ...prefs, [kind]: on };
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(prefs));
  } catch {
    /* storage unavailable */
  }
  prefListeners.forEach((l) => l());
}

if (typeof window !== "undefined") {
  window.addEventListener("storage", (e) => {
    if (e.key !== PREFS_KEY) return;
    prefs = readPrefs();
    prefListeners.forEach((l) => l());
  });
}

/** The everyday sound switches, kept in sync across tabs. */
export function useSoundPrefs(): Record<SoundPref, boolean> {
  return useSyncExternalStore(
    (cb) => {
      prefListeners.add(cb);
      return () => prefListeners.delete(cb);
    },
    () => prefs,
    () => prefs,
  );
}

// ── The on/off preference, per browser ─────────────────────────────────────────────

const KEY = "od-chat-alert-sound";
const listeners = new Set<() => void>();

function read(): boolean {
  try {
    return localStorage.getItem(KEY) !== "off";
  } catch {
    return true;
  }
}

let enabled = read();

export function setAlertSound(on: boolean) {
  enabled = on;
  try {
    localStorage.setItem(KEY, on ? "on" : "off");
  } catch {
    /* storage unavailable */
  }
  listeners.forEach((l) => l());
}

export function alertSoundOn(): boolean {
  return enabled;
}

if (typeof window !== "undefined") {
  window.addEventListener("storage", (e) => {
    if (e.key !== KEY) return;
    enabled = read();
    listeners.forEach((l) => l());
  });
}

/** Whether alert sounds are on, kept in sync across tabs. */
export function useAlertSound(): boolean {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => enabled,
    () => true,
  );
}

// ── One tab plays ──────────────────────────────────────────────────────────────────

const PLAYED_KEY = "od-chat-alert-played";

function allowed(kind: AlertSound): boolean {
  return kind === "waiting" || kind === "reminder" ? enabled : prefs[kind];
}

// A burst (a mailbox catching up, a bulk assign) plays one everyday sound, not a stream.
const EVERYDAY_GAP_MS = 2000;
let lastEveryday = 0;

/** Plays unless it is switched off or another tab of this browser just played the same alert. */
export function playAlertOnce(kind: AlertSound, id: string) {
  if (!allowed(kind)) return;
  if (kind !== "waiting" && kind !== "reminder") {
    if (Date.now() - lastEveryday < EVERYDAY_GAP_MS) return;
    lastEveryday = Date.now();
  }
  const stamp = `${kind}:${id}`;
  try {
    const last = localStorage.getItem(PLAYED_KEY);
    if (last) {
      const [at, prev] = [Number(last.slice(0, last.indexOf("|"))), last.slice(last.indexOf("|") + 1)];
      if (prev === stamp && Date.now() - at < 5000) return;
    }
    localStorage.setItem(PLAYED_KEY, `${Date.now()}|${stamp}`);
  } catch {
    /* storage unavailable: play anyway */
  }
  playAlert(kind);
}
