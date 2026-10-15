import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { createMemoryHistory, createRootRoute, createRouter, RouterContextProvider } from "@tanstack/react-router";
import "@/i18n";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { BotPersonality } from "@/lib/api/types";
import { OLLIE_LOOK, readLook, type BotLook } from "@/lib/botlook";
import { BotFace } from "./BotFace";
import { LookEditor } from "./LookEditor";
import { PersonalityEditor } from "./PersonalityEditor";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const router = createRouter({ routeTree: createRootRoute(), history: createMemoryHistory() });
const PERSONALITY: BotPersonality = {
  tone: "friendly",
  custom_tone: "",
  reply_length: "short",
  emoji: "none",
  greeting: "",
  handoff_message: "",
  sign_off: "",
  instructions: "",
  blocked_topics: [],
};

let root: Root;
let el: HTMLDivElement;
let look: BotLook;
let personality: BotPersonality;

function LookHarness() {
  const [l, setL] = useState<BotLook>({ ...OLLIE_LOOK });
  look = l;
  return (
    <TooltipProvider>
      <LookEditor look={l} onChange={setL} name="Pip" image={null} onUpload={async () => {}} />
      <BotFace look={l} size={96} label="Preview" />
    </TooltipProvider>
  );
}

function PersonalityHarness() {
  const [p, setP] = useState<BotPersonality>(PERSONALITY);
  const [lang, setLang] = useState("");
  personality = p;
  return (
    <RouterContextProvider router={router}>
      <TooltipProvider>
        <PersonalityEditor value={p} onChange={setP} language={lang} onLanguage={setLang} />
      </TooltipProvider>
    </RouterContextProvider>
  );
}

const radio = (group: string, name: string) => el.querySelector<HTMLButtonElement>(`[role=radiogroup][aria-label="${group}"] [role=radio][aria-label="${name}"]`)!;
const byText = (sel: string, text: string) => [...el.querySelectorAll<HTMLElement>(sel)].find((b) => b.textContent?.trim().startsWith(text))!;
const preview = () => el.querySelector('[aria-label="Preview"]')!.innerHTML;
const type = (input: HTMLInputElement | HTMLTextAreaElement, value: string) => {
  let proto = Object.getPrototypeOf(input);
  while (proto && !Object.getOwnPropertyDescriptor(proto, "value")) proto = Object.getPrototypeOf(proto);
  Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
};

beforeEach(() => {
  el = document.createElement("div");
  document.body.appendChild(el);
  root = createRoot(el);
});
afterEach(() => {
  act(() => root.unmount());
  el.remove();
});

describe("look builder", () => {
  it("swaps parts from the tiles and redraws the face", async () => {
    await act(async () => root.render(<LookHarness />));
    const before = preview();
    expect(radio("Eyes", "Pills").getAttribute("aria-checked")).toBe("true");
    await act(async () => radio("Eyes", "Glasses").click());
    expect(look.eyes).toBe("glasses");
    expect(radio("Eyes", "Glasses").getAttribute("aria-checked")).toBe("true");
    expect(preview()).not.toBe(before);
    await act(async () => radio("Accessory", "Crown").click());
    await act(async () => radio("Shape", "Hexagon").click());
    expect(look).toMatchObject({ shape: "hexagon", accessory: "crown" });
    // Every tile is a whole face with that one part changed.
    expect(radio("Mouth", "Grin").querySelector("svg")).not.toBeNull();
  });

  it("moves through parts with the arrow keys", async () => {
    await act(async () => root.render(<LookHarness />));
    const blob = radio("Shape", "Blob");
    expect(blob.tabIndex).toBe(0);
    expect(radio("Shape", "Circle").tabIndex).toBe(-1);
    await act(async () => blob.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })));
    expect(look.shape).toBe("circle");
    await act(async () => radio("Shape", "Circle").dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true })));
    expect(look.shape).toBe("cloud");
  });

  it("offers an accent only for accessories that use it", async () => {
    await act(async () => root.render(<LookHarness />));
    expect(el.querySelector('[role=radiogroup][aria-label="Accent"]')).toBeNull();
    await act(async () => radio("Accessory", "Crown").click());
    expect(el.querySelector('[role=radiogroup][aria-label="Accent"]')).not.toBeNull();
    await act(async () => radio("Accessory", "Halo").click());
    expect(el.querySelector('[role=radiogroup][aria-label="Accent"]')).toBeNull();
  });

  it("switches to an emoji on a shape and to a gradient fill", async () => {
    await act(async () => root.render(<LookHarness />));
    await act(async () => byText("button[role=radio]", "Emoji").click());
    expect(look.kind).toBe("emoji");
    expect(el.querySelector('[role=radiogroup][aria-label="Eyes"]')).toBeNull();
    expect(preview()).toContain("🦊");
    await act(async () => byText("button[role=radio]", "Gradient").click());
    await act(async () => radio("Colour", "Teal").click());
    expect(look.fill).toEqual({ type: "gradient", color: "#9ff0e3", to: "#0b7f73" });
    // The renderer only accepts what readLook would keep.
    expect(readLook(look)).toEqual(look);
  });

  it("takes a custom colour once it is a full hex", async () => {
    await act(async () => root.render(<LookHarness />));
    const input = el.querySelector<HTMLInputElement>('input[aria-label="From"]')!;
    await act(async () => type(input, "#12"));
    expect(look.fill.color).toBe(OLLIE_LOOK.fill.color);
    expect(input.getAttribute("aria-invalid")).toBe("true");
    await act(async () => type(input, "#12AB34"));
    expect(look.fill.color).toBe("#12ab34");
  });
});

describe("personality editor", () => {
  it("picks a tone from the cards, with arrow keys too", async () => {
    await act(async () => root.render(<PersonalityHarness />));
    const card = (name: string) => [...el.querySelectorAll<HTMLButtonElement>('[aria-label="Tone"] [role=radio]')].find((b) => b.textContent?.startsWith(name))!;
    expect(card("Friendly").getAttribute("aria-checked")).toBe("true");
    await act(async () => card("Playful").click());
    expect(personality.tone).toBe("playful");
    await act(async () => card("Playful").dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true })));
    expect(personality.tone).toBe("concise");
  });

  it("asks for words when the tone is custom", async () => {
    await act(async () => root.render(<PersonalityHarness />));
    await act(async () => [...el.querySelectorAll<HTMLButtonElement>('[aria-label="Tone"] [role=radio]')].find((b) => b.textContent?.startsWith("Custom"))!.click());
    const field = el.querySelector<HTMLInputElement>("#bot-custom-tone")!;
    expect(field).not.toBeNull();
    expect(el.textContent).toContain("Required");
    await act(async () => type(field, "Calm and nerdy"));
    expect(personality.custom_tone).toBe("Calm and nerdy");
    expect(el.querySelector("#bot-custom-tone-err")).toBeNull();
  });

  it("refuses markup in what it says", async () => {
    await act(async () => root.render(<PersonalityHarness />));
    await act(async () => type(el.querySelector<HTMLTextAreaElement>("#bot-greeting")!, "Hi <b>there</b>"));
    expect(el.querySelector("#bot-greeting-err")?.textContent).toBe("Plain text only, without < or >");
    await act(async () => type(el.querySelector<HTMLTextAreaElement>("#bot-greeting")!, "Hi there!\nAsk me anything."));
    expect(el.querySelector("#bot-greeting-err")).toBeNull();
  });

  it("sets reply length and emoji use", async () => {
    await act(async () => root.render(<PersonalityHarness />));
    await act(async () => byText("button[role=radio]", "Detailed").click());
    await act(async () => byText("button[role=radio]", "Lots").click());
    expect(personality).toMatchObject({ reply_length: "detailed", emoji: "lots" });
    expect(el.textContent).toContain("Freely, to keep it light.");
  });
});

describe("bot face", () => {
  it("shows an uploaded picture only for a picture look", () => {
    act(() => root.render(<BotFace look={{ ...OLLIE_LOOK, kind: "image" }} image="https://cdn.example/pip.png" label="Pip" size={40} />));
    expect(el.querySelector("img")?.getAttribute("src")).toBe("https://cdn.example/pip.png");
    act(() => root.render(<BotFace look={OLLIE_LOOK} image="https://cdn.example/pip.png" label="Pip" size={40} state="thinking" blink />));
    expect(el.querySelector("img")).toBeNull();
    const face = el.querySelector('[role="img"]')!;
    expect(face.getAttribute("aria-label")).toBe("Pip");
    expect(face.getAttribute("data-bot-state")).toBe("thinking");
    expect(face.className).toContain("odb-anim");
    expect(document.head.querySelector("style[data-od-bot-look]")?.textContent).toContain("prefers-reduced-motion");
  });

  it("draws a safe face from a tampered look", () => {
    act(() => root.render(<BotFace look={{ kind: "built", shape: "<script>", fill: { type: "solid", color: "red" }, emoji: "<img onerror=x>" }} label="X" />));
    const html = el.innerHTML;
    expect(html).not.toContain("<script");
    expect(html).not.toContain("onerror");
    expect(el.querySelector("svg")).not.toBeNull();
  });
});
