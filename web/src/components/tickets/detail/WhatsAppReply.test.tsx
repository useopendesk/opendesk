import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import "@/i18n";
import { i18n } from "@/i18n";
import type { WhatsAppTemplate, WhatsAppTicketState } from "@/lib/api/types";
import { fillTemplate, WhatsAppLine, WhatsAppTemplatePanel, type WhatsAppReply } from "./WhatsAppReply";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement;
let root: Root;

beforeEach(async () => {
  await i18n.changeLanguage("en");
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
});

const template: WhatsAppTemplate = { id: "HX1", name: "follow_up", language: "en", body: "Hi {{1}}, about {{2}}?", variables: ["1", "2"] };

function reply(over: Partial<WhatsAppReply> = {}): WhatsAppReply {
  const state: WhatsAppTicketState = {
    connection: { id: "c1", name: "Margin", sender: "+14155550100" },
    window: { open: true, closes_at: new Date(Date.now() + 5 * 3600_000).toISOString() },
  };
  return { active: true, state, disconnected: false, closed: false, templates: [template], loadingTemplates: false, pick: null, setPick: () => {}, ready: false, html: "", sendable: true, ...over };
}

const show = (node: React.ReactNode) => act(async () => root.render(<QueryClientProvider client={new QueryClient()}>{node}</QueryClientProvider>));

describe("WhatsApp reply", () => {
  it("fills a template's variables", () => {
    expect(fillTemplate("Hi {{1}}, about {{ 2 }}?", { "1": "Ana", "2": "your invoice" })).toBe("Hi Ana, about your invoice?");
    expect(fillTemplate("Hi {{1}}", {})).toBe("Hi ");
  });

  it("says where the reply goes and how long it can be free-form", async () => {
    await show(<WhatsAppLine ticket={{ contact: { id: "x", email: null, name: "Mateo", avatar_url: null } as never }} wa={reply()} />);
    expect(container.textContent).toContain("Mateo");
    expect(container.textContent).toContain("from +14155550100");
    expect(container.textContent).toMatch(/reply freely for \d+ h/);
  });

  it("says the window is closed, and when the number is no longer connected", async () => {
    const state = { connection: { id: "c1", name: "Margin", sender: "+1" }, window: { open: false, closes_at: null } };
    await show(<WhatsAppLine ticket={{ contact: null }} wa={reply({ closed: true, state })} />);
    expect(container.textContent).toContain("24-hour window closed");
    await show(<WhatsAppLine ticket={{ contact: null }} wa={reply({ disconnected: true, state: { connection: null, window: { open: false, closes_at: null } } })} />);
    expect(container.textContent).toContain("Number not connected");
  });

  it("offers templates only when the window is closed, with a preview once filled", async () => {
    await show(<WhatsAppTemplatePanel wa={reply()} />);
    expect(container.querySelector('[data-testid="whatsapp-templates"]')).toBeNull();
    await show(<WhatsAppTemplatePanel wa={reply({ closed: true })} />);
    expect(container.textContent).toContain("only allows an approved template");
    await show(
      <WhatsAppTemplatePanel
        wa={reply({ closed: true, template, ready: true, pick: { id: "HX1", variables: { "1": "Ana", "2": "your invoice" } } })}
      />,
    );
    expect(container.querySelectorAll("input")).toHaveLength(2);
    expect(container.querySelector('[data-testid="whatsapp-preview"]')?.textContent).toBe("Hi Ana, about your invoice?");
  });

  it("explains an account without approved templates", async () => {
    await show(<WhatsAppTemplatePanel wa={reply({ closed: true, templates: [] })} />);
    expect(container.textContent).toContain("no approved templates");
  });
});
