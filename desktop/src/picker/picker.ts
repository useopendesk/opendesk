// The server picker page (and the offline page). No network access: main
// checks the address and opens the instance.

type ChooseResult = { ok: true; host: string } | { ok: false; error: string; host?: string };

declare global {
  interface Window {
    picker?: {
      info: { platform: string; cloudOrigin: string; current: string | null; canCancel: boolean };
      choose(address: string): Promise<ChooseResult>;
      cancel(): void;
    };
  }
}

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;

function errorCopy(code: string, host?: string): string {
  const h = host ?? "That server";
  switch (code) {
    case "empty":
      return "Enter your server's address.";
    case "invalid":
      return "That doesn't look like a web address. Try something like support.example.com.";
    case "insecure":
      return "opendesk connects over https only. Use an https:// address.";
    case "timeout":
      return `${h} took too long to answer. Try again in a moment.`;
    case "certificate":
      return `${h} has a certificate problem, so the connection isn't safe.`;
    case "redirected":
      return `${h} sends visitors to another address. Use that address instead.`;
    case "not_opendesk":
      return `${h} doesn't look like an opendesk server. Use the address you open opendesk at in your browser.`;
    default:
      return `Couldn't reach ${h}. Check the address and your connection, then try again.`;
  }
}

function initPicker() {
  const api = window.picker;
  if (!api) return;
  const info = api.info;
  if (info.platform === "linux") document.documentElement.classList.add("opaque");

  const form = $<HTMLFormElement>("form");
  const wrap = $("address-wrap");
  const input = $<HTMLInputElement>("address");
  const error = $("error");
  const submit = $<HTMLButtonElement>("continue");
  const cancel = $<HTMLButtonElement>("cancel");
  const radios = [...document.querySelectorAll<HTMLInputElement>('input[name="kind"]')];

  const kind = () => radios.find((r) => r.checked)?.value ?? "cloud";
  const showError = (msg: string | null) => {
    error.hidden = !msg;
    error.textContent = msg ?? "";
    input.setAttribute("aria-invalid", msg && kind() === "own" ? "true" : "false");
  };
  const sync = () => {
    wrap.hidden = kind() !== "own";
    showError(null);
    if (kind() === "own") requestAnimationFrame(() => input.focus());
  };
  radios.forEach((r) => r.addEventListener("change", sync));
  input.addEventListener("input", () => showError(null));

  // Switching server: start from the current one.
  if (info.current && info.current !== info.cloudOrigin) {
    radios.find((r) => r.value === "own")!.checked = true;
    input.value = new URL(info.current).host;
    wrap.hidden = false;
  }
  if (info.canCancel) {
    cancel.hidden = false;
    cancel.addEventListener("click", () => api.cancel());
  }
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && info.canCancel) api.cancel();
  });

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (submit.classList.contains("busy")) return;
    const address = kind() === "cloud" ? info.cloudOrigin : input.value;
    if (kind() === "own" && !input.value.trim()) {
      showError(errorCopy("empty"));
      input.focus();
      return;
    }
    showError(null);
    submit.classList.add("busy");
    submit.querySelector(".label")!.textContent = "Checking";
    try {
      const r = await api.choose(address);
      if (!r.ok) {
        showError(errorCopy(r.error, r.host));
        if (kind() === "own") input.focus();
      }
    } catch {
      showError(errorCopy("unreachable"));
    } finally {
      submit.classList.remove("busy");
      submit.querySelector(".label")!.textContent = "Continue";
    }
  });
}

function initOffline() {
  const host = new URLSearchParams(location.search).get("host");
  if (host && /^[a-z0-9.:[\]-]{1,255}$/i.test(host)) $("host").textContent = host;
  if (navigator.platform.toLowerCase().startsWith("linux")) document.documentElement.classList.add("opaque");
}

if (document.body.dataset.page === "offline") initOffline();
else initPicker();

export {};
