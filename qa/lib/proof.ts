import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { test as base, expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import { assertLocal, RUN_DIR } from "./env.ts";
import { Recorder } from "./recorder.ts";
import { serve, stopAll, type Target } from "./servers.ts";

export type Box = { x: number; y: number; width: number; height: number };
// `ignore` holds regions, in the still's own pixels, that comparisons skip.
export type Shot = { name: string; file: string; caption: string; ignore?: Box[] };

export type FlowMeta = {
  slug: string;
  title: string;
  file: string;
  target: Target;
  status: TestInfo["status"];
  // Spooled frames waiting for the end-of-run encode; `video` replaces it once encoded.
  frames?: string;
  video?: string;
  shots: Shot[];
};

// The same instant and the same random sequence on every run, so the mock seeds identical data and
// a follow-up's stills differ from the last publish only where the code changed.
const FIXED_TIME = new Date("2026-06-16T09:30:00Z");
const SEEDED_RANDOM = `(() => {
  let a = 0x0d5eed;
  Math.random = () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
})();`;

// The animated pointer, click ripples and action labels in the video; never in a still.
const ACTIONS = { cursor: "pointer", duration: 700, position: "top-right", style: { title: "font-size: 16px" } } as const;

// Where `open()` lands when given no path.
const HOME: Record<Target, string> = { web: "/", widget: "/widget/demo.html", site: "/", docs: "/" };

export class Proof {
  readonly shots: Shot[] = [];
  readonly dir: string;
  private readonly page: Page;
  private readonly target: Target;
  private readonly live: boolean;

  constructor(page: Page, dir: string, target: Target, live: boolean) {
    this.page = page;
    this.dir = dir;
    this.target = target;
    this.live = live;
  }

  // Opens a path on the flow's target. On the app, the mock's simulated activity stays off
  // (`test.use({ live: true })` turns it on) so the screen holds still for the stills.
  async open(path = HOME[this.target], params: Record<string, string> = {}): Promise<void> {
    const url = new URL(path, "http://x");
    if (this.target === "web" && this.live) url.searchParams.set("mock_live", "1");
    for (const [k, v] of Object.entries(params)) url.searchParams.set(k, v);
    await this.page.goto(`${url.pathname}${url.search}${url.hash}`);
  }

  // A title card in the video, so a reviewer knows what the next few seconds show.
  async chapter(title: string, description?: string): Promise<void> {
    await this.page.screencast.showChapter(title, { description, duration: 1600 });
  }

  // A named still. Keep names stable: follow-up comments diff stills by name.
  // `ignore` names content that changes on its own (relative times, live counters): the still is
  // published as is, and those regions are skipped when it is compared with the previous publish.
  async shot(name: string, opts: { caption?: string; target?: Locator; ignore?: Locator[] } = {}): Promise<void> {
    if (!/^[a-z0-9][a-z0-9-]*$/.test(name)) throw new Error(`shot name "${name}" must be kebab-case`);
    if (this.shots.some((s) => s.name === name)) throw new Error(`shot "${name}" taken twice in one flow`);
    const file = join(this.dir, "shots", `${name}.png`);
    const origin = opts.target ? await opts.target.boundingBox() : { x: 0, y: 0 };
    const ignore: Box[] = [];
    for (const locator of opts.ignore ?? []) {
      for (const el of await locator.all()) {
        const b = await el.boundingBox();
        if (b && origin) ignore.push({ x: Math.floor(b.x - origin.x), y: Math.floor(b.y - origin.y), width: Math.ceil(b.width) + 1, height: Math.ceil(b.height) + 1 });
      }
    }
    const options = { path: file, animations: "disabled" as const, caret: "hide" as const };
    await this.page.screencast.hideOverlays();
    await this.page.screencast.hideActions();
    try {
      if (opts.target) await opts.target.screenshot(options);
      else await this.page.screenshot(options);
    } finally {
      await this.page.screencast.showOverlays();
      await this.page.screencast.showActions(ACTIONS);
    }
    this.shots.push({ name, file, caption: opts.caption ?? name, ignore: ignore.length ? ignore : undefined });
  }

  // Lets a result sit on screen long enough to be read in the video.
  async dwell(ms = 1200): Promise<void> {
    await this.page.waitForTimeout(ms);
  }
}

export function slugOf(info: TestInfo): string {
  return info.titlePath
    .slice(1)
    .join(" ")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

export const test = base.extend<{ proof: Proof; target: Target; live: boolean; fixedClock: boolean }, { servers: (t: Target) => Promise<string> }>({
  // `test.use({ target: "site" })` for a flow on the marketing site; also "widget" and "docs".
  target: ["web", { option: true }],
  // `test.use({ live: true })` keeps the mock's simulated customers, teammates and Ollie running.
  live: [false, { option: true }],
  // `test.use({ fixedClock: false })` lets time run, for a flow about timers or countdowns.
  fixedClock: [true, { option: true }],
  servers: [
    async ({}, use) => {
      await use(serve);
      stopAll();
    },
    { scope: "worker" },
  ],
  baseURL: async ({ target, servers }, use) => {
    const url = await servers(target);
    assertLocal(url);
    await use(url);
  },
  proof: [
    async ({ page, target, live, fixedClock }, use, info) => {
      const slug = slugOf(info);
      const dir = join(RUN_DIR, slug);
      mkdirSync(join(dir, "shots"), { recursive: true });
      if (fixedClock) await page.clock.setFixedTime(FIXED_TIME);
      if (target === "web" || target === "widget") await page.addInitScript(SEEDED_RANDOM);

      const frames = join(dir, "frames");
      const recorder = new Recorder(page, frames);
      await recorder.start();
      await page.screencast.showActions(ACTIONS);
      const proof = new Proof(page, dir, target, live);

      await use(proof);

      const captured = await recorder.stop();
      const stills = proof.shots.length > 0;
      const status = info.status === "passed" && !stills ? "failed" : info.status;
      // A failed flow is never published, so its frames are not worth encoding unless asked for.
      const keep = captured && (status === "passed" || process.env.QA_VIDEO_ON_FAIL === "1");
      if (!keep) rmSync(frames, { recursive: true, force: true });
      const meta: FlowMeta = { slug, title: info.title, file: info.file, target, status, frames: keep ? frames : undefined, shots: proof.shots };
      writeFileSync(join(dir, "meta.json"), JSON.stringify(meta, null, 2));
      if (!stills && info.status === "passed") throw new Error(`"${info.title}" took no stills; every flow publishes at least one \`proof.shot\``);
    },
    { auto: true },
  ],
});

export { expect };
