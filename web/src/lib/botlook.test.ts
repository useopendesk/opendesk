import { describe, expect, it } from "vitest";
import fixtures from "./botlook.fixtures.json";
import {
  BOT_ACCESSORIES,
  BOT_EYES,
  BOT_MOUTHS,
  BOT_PALETTE,
  BOT_SHAPES,
  inkFor,
  isEmoji,
  lookProblem,
  OLLIE_LOOK,
  randomLook,
  readLook,
  renderBotSvg,
  type BotLook,
} from "./botlook";

const parse = (svg: string) => new DOMParser().parseFromString(svg, "image/svg+xml");

describe("renderBotSvg", () => {
  it("matches the shared fixtures the Go renderer is checked against", () => {
    for (const f of fixtures as { look: BotLook; svg: string }[]) expect(renderBotSvg(f.look)).toBe(f.svg);
  });

  it("draws Ollie as a violet gradient blob with two eyes and no mouth", () => {
    const doc = parse(renderBotSvg(OLLIE_LOOK));
    expect(doc.querySelector("parsererror")).toBeNull();
    expect(doc.querySelector("svg")?.getAttribute("viewBox")).toBe("0 0 64 64");
    expect(doc.querySelector("radialGradient stop")?.getAttribute("stop-color")).toBe("#c9bcff");
    expect(doc.querySelector(".odb-body")?.getAttribute("fill")).toBe("url(#odb-g-c9bcff-4a2bc4)");
    expect(doc.querySelector(".odb-eyes")?.getAttribute("fill")).toBe("#ffffff");
    expect(doc.querySelector(".odb-mouth")).toBeNull();
    expect(doc.querySelector(".odb-acc")).toBeNull();
  });

  it("renders every part as well-formed SVG with its own geometry", () => {
    const seen = new Set<string>();
    const check = (look: BotLook, sel: string) => {
      const doc = parse(renderBotSvg(look));
      expect(doc.querySelector("parsererror")).toBeNull();
      const d = [...doc.querySelectorAll(`${sel} path`)].map((p) => p.getAttribute("d")).join("|") || doc.querySelector(sel)?.getAttribute("d") || "";
      expect(d).not.toBe("");
      expect(seen.has(d)).toBe(false);
      seen.add(d);
    };
    for (const shape of BOT_SHAPES) check({ ...OLLIE_LOOK, shape }, ".odb-body");
    for (const eyes of BOT_EYES) check({ ...OLLIE_LOOK, eyes }, ".odb-eyes");
    for (const mouth of BOT_MOUTHS.filter((m) => m !== "none")) check({ ...OLLIE_LOOK, mouth }, ".odb-mouth");
    for (const accessory of BOT_ACCESSORIES.filter((a) => a !== "none")) check({ ...OLLIE_LOOK, accessory }, ".odb-acc");
  });

  it("makes room for an accessory and a background", () => {
    expect(renderBotSvg(OLLIE_LOOK)).not.toContain("transform=");
    expect(renderBotSvg({ ...OLLIE_LOOK, accessory: "crown" })).toContain('transform="matrix(.9 0 0 .9 3.2 6.6)"');
    expect(renderBotSvg({ ...OLLIE_LOOK, background: "soft" })).toContain('opacity=".16"');
    expect(renderBotSvg({ ...OLLIE_LOOK, background: "ring", accessory: "halo" })).toContain("matrix(.702 0 0 .702 9.536 12.188)");
  });

  it("draws an emoji face escaped, without eyes or accessories", () => {
    const svg = renderBotSvg({ ...OLLIE_LOOK, kind: "emoji", emoji: "🦊", accessory: "crown" });
    const doc = parse(svg);
    expect(doc.querySelector("text")?.textContent).toBe("🦊");
    expect(doc.querySelector(".odb-eyes")).toBeNull();
    expect(doc.querySelector(".odb-acc")).toBeNull();
    // A tampered look never becomes markup.
    expect(renderBotSvg({ ...OLLIE_LOOK, kind: "emoji", emoji: "<script>" })).not.toContain("<script>");
  });

  it("picks ink that reads on the fill", () => {
    expect(inkFor({ type: "solid", color: "#ffffff", to: "#ffffff" })).toBe("#2a2838");
    expect(inkFor({ type: "solid", color: "#2b2a35", to: "#2b2a35" })).toBe("#ffffff");
    expect(inkFor(OLLIE_LOOK.fill)).toBe("#ffffff");
  });
});

describe("randomLook", () => {
  it("is deterministic for a seed and varied across seeds", () => {
    expect(randomLook(42)).toEqual(randomLook(42));
    const many = new Set(Array.from({ length: 40 }, (_, i) => JSON.stringify(randomLook(i + 1))));
    expect(many.size).toBeGreaterThan(35);
  });

  it("only rolls valid looks from the curated palette", () => {
    const colours = new Set<string>(BOT_PALETTE.flatMap((p) => [p.base, p.light, p.deep]));
    for (let i = 0; i < 300; i++) {
      const l = randomLook(i * 31 + 7);
      expect(lookProblem(l)).toBeNull();
      expect(l.kind).toBe("built");
      expect(colours.has(l.fill.color)).toBe(true);
      expect(l.accent).not.toBe(l.fill.color);
      if (l.eyes === "sleepy") expect(["none", "tiny", "flat", "smile"]).toContain(l.mouth);
    }
  });
});

describe("validation", () => {
  it("accepts Ollie and refuses anything off the allow-list", () => {
    expect(lookProblem(OLLIE_LOOK)).toBeNull();
    expect(lookProblem({ ...OLLIE_LOOK, svg: "<svg/>" })).toMatch(/not a look setting/);
    expect(lookProblem({ ...OLLIE_LOOK, shape: "star" })).toMatch(/shape/);
    expect(lookProblem({ ...OLLIE_LOOK, accent: "red" })).toMatch(/accent/);
    expect(lookProblem({ ...OLLIE_LOOK, fill: { type: "solid", color: "url(#x)", to: "#000000" } })).toMatch(/fill.color/);
    expect(lookProblem({ ...OLLIE_LOOK, kind: "emoji", emoji: "" })).toMatch(/emoji/);
    const { blush: _, ...missing } = OLLIE_LOOK;
    expect(lookProblem(missing)).toMatch(/blush/);
  });

  it("knows an emoji from text", () => {
    expect(isEmoji("🦊")).toBe(true);
    expect(isEmoji("👩🏽‍🚀")).toBe(true);
    expect(isEmoji("1️⃣")).toBe(true);
    expect(isEmoji("hi")).toBe(false);
    expect(isEmoji("<b>")).toBe(false);
    expect(isEmoji("🦊🦊🦊🦊🦊🦊🦊🦊🦊🦊🦊")).toBe(false);
  });

  it("reads stored looks leniently for display", () => {
    expect(readLook(null)).toEqual(OLLIE_LOOK);
    expect(readLook({ shape: "cloud", accent: "#ABCDEF" })).toMatchObject({ shape: "cloud", accent: "#abcdef", eyes: "pills" });
  });
});
