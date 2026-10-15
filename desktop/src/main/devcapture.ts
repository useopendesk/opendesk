// Development only: screenshots of the instance window for design review when
// the OS screen capture is unavailable. `--capture=<dir>` saves the page after it
// loads, then after each `--capture-paths=/a;;/b`. Each shot is also composited
// over a stand-in for the macOS sidebar material with the traffic lights drawn
// where the window puts them, since a page capture has neither.

import { BrowserWindow } from "electron";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { devArg } from "./env";
import type { InstanceWindow } from "./window";

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function composite(png: Buffer, width: number, height: number, dark: boolean, lightsAt = { x: 18, y: 17 }): Promise<Buffer> {
  const w = new BrowserWindow({ width, height, show: false, frame: false, transparent: true, webPreferences: { offscreen: true, sandbox: true, contextIsolation: true } });
  const src = `data:image/png;base64,${png.toString("base64")}`;
  const bg = dark
    ? "radial-gradient(120% 90% at 10% 0%, #3a3a46 0%, #26262c 45%, #1e1e22 100%)"
    : "radial-gradient(120% 90% at 10% 0%, #e6e8f0 0%, #ececee 45%, #e9e9ea 100%)";
  const lights = ["#ff5f57", "#febc2e", "#28c840"]
    .map((c, i) => `<i style="position:absolute;top:${lightsAt.y + 1}px;left:${lightsAt.x + i * 20}px;width:12px;height:12px;border-radius:50%;background:${c};box-shadow:inset 0 0 0 0.5px rgb(0 0 0/.15)"></i>`)
    .join("");
  const html = `<html><body style="margin:0;width:${width}px;height:${height}px;background:${bg};position:relative;overflow:hidden"><img src="${src}" style="position:absolute;inset:0;width:${width}px;height:${height}px">${lights}</body></html>`;
  await w.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`);
  await wait(400);
  const img = await w.webContents.capturePage();
  w.destroy();
  return img.toPNG();
}

export function setupDevCapture(c: InstanceWindow, quit: () => void) {
  const dir = devArg("capture");
  if (!dir) return;
  const paths = (devArg("capture-paths") ?? "").split(";;").filter(Boolean);
  const name = devArg("capture-name") ?? "shot";
  mkdirSync(dir, { recursive: true });
  let done = false;
  c.win.webContents.on("did-finish-load", async () => {
    if (done || !c.win.webContents.getURL().startsWith(c.instance.appOrigin)) return;
    // With --theme, the first load only stores the theme and reloads.
    const theme = devArg("theme");
    if (theme && (await c.win.webContents.executeJavaScript(`localStorage.getItem("od-theme")`)) !== theme) return;
    done = true;
    await wait(3500);
    const shoot = async (label: string) => {
      const img = await c.win.webContents.capturePage();
      const [width, height] = c.win.getContentSize();
      const dark = await c.win.webContents.executeJavaScript("document.documentElement.classList.contains('dark')");
      writeFileSync(join(dir, `${name}-${label}-page.png`), img.toPNG());
      writeFileSync(join(dir, `${name}-${label}.png`), await composite(img.toPNG(), width, height, dark));
      console.log(`captured ${name}-${label}`);
    };
    await shoot("start");
    // Each entry is `path` or `path|script`; the script runs before the shot (clicks, typing).
    for (const [i, entry] of paths.entries()) {
      const [p, script] = entry.split("|");
      c.navigate(p);
      await wait(2500);
      if (script) {
        await c.win.webContents.executeJavaScript(script).catch((e) => console.log("capture script:", String(e)));
        await wait(1200);
      }
      await shoot(String(i + 1));
    }
    if (devArg("capture-quit") === "1") quit();
  });
}

/** The server picker: `--capture=<dir>` with `--picker=1`. */
export function capturePicker(win: BrowserWindow, quit: () => void) {
  const dir = devArg("capture");
  if (!dir) return;
  mkdirSync(dir, { recursive: true });
  win.webContents.once("did-finish-load", async () => {
    await wait(1200);
    const name = devArg("capture-name") ?? "picker";
    const shoot = async (label: string) => {
      const img = await win.webContents.capturePage();
      const [width, height] = win.getContentSize();
      const dark = await win.webContents.executeJavaScript("matchMedia('(prefers-color-scheme: dark)').matches");
      writeFileSync(join(dir, `${name}-${label}.png`), await composite(img.toPNG(), width, height, dark, { x: 20, y: 18 }));
      console.log(`captured ${name}-${label}`);
    };
    await shoot("cloud");
    const script = devArg("capture-script");
    if (script) {
      await win.webContents.executeJavaScript(script).catch((e) => console.log("capture script:", String(e)));
      await wait(4000);
      await shoot("own");
    }
    if (devArg("capture-quit") === "1") quit();
  });
}
