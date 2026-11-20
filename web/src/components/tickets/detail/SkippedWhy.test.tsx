import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import "@/i18n";
import { SkippedWhy } from "./Timeline";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

async function open(data: Record<string, unknown>) {
  await act(async () => root.render(<SkippedWhy data={data} />));
  const button = container.querySelector("button");
  if (button) await act(async () => button.click());
  return button;
}

describe("SkippedWhy", () => {
  it("says why the agent left the message", async () => {
    await open({ reason: "monthly_cap" });
    const text = document.body.textContent ?? "";
    expect(text).toContain("monthly cap");
    expect(text).toContain("Only the team sees this.");
  });

  it("shows nothing for an unknown reason", async () => {
    expect(await open({})).toBeNull();
    expect(await open({ reason: "something_new" })).toBeNull();
  });
});
