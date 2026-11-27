import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import "@/i18n";
import type { ReactNode } from "react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { api } from "@/lib/api/endpoints";
import type { Page, Ticket, TicketLink, TicketRelation } from "@/lib/api/types";
import { qk } from "@/lib/queries";
import { TicketLinks } from "./TicketLinks";

vi.mock("@tanstack/react-router", async (orig) => ({
  ...(await orig<object>()),
  Link: ({ children, params, className }: { children: ReactNode; params: { ticketId: string }; className?: string }) => (
    <a href={`/inbox/open/${params.ticketId}`} className={className}>
      {children}
    </a>
  ),
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

let host: HTMLDivElement;
let root: ReturnType<typeof createRoot>;
let client: QueryClient;

const addedLink: TicketLink = {
  id: "link-1",
  ticket_id: "ticket-1",
  integration_id: null,
  provider: "linear",
  external_id: null,
  external_state: null,
  external_state_name: null,
  synced_at: null,
  url: "https://linear.app/acme/issue/ENG-123",
  title: "ENG-123",
  actor_type: "user",
  created_by: "user-1",
  created_at: "2026-11-02T12:00:00Z",
};

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(qk.ticketLinks("ticket-1"), []);
  client.setQueryData(qk.ticketRelations("ticket-1"), []);
});

afterEach(async () => {
  await act(async () => root.unmount());
  client.clear();
  document.body.innerHTML = "";
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function render(canEdit = true) {
  await act(async () => {
    root.render(
      <QueryClientProvider client={client}>
        <TooltipProvider>
          <TicketLinks ticketId="ticket-1" canEdit={canEdit} />
        </TooltipProvider>
      </QueryClientProvider>,
    );
  });
}

function input(index: number) {
  return document.querySelectorAll<HTMLInputElement>("[role='dialog'] input")[index];
}

async function type(target: HTMLInputElement, value: string) {
  const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  await act(async () => {
    set.call(target, value);
    target.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

it("lets an agent add a provider link and shows it on the ticket", async () => {
  const addLink = vi.spyOn(api.tickets, "addLink").mockResolvedValue(addedLink);
  await render();

  const addButton = [...host.querySelectorAll("button")].find((button) => button.textContent?.trim() === "Add link")!;
  await act(async () => addButton.click());
  expect(document.querySelector("[role='dialog']")?.textContent).toContain("Add a link");

  await type(input(0), addedLink.url);
  await type(input(1), " ENG-123 ");
  await act(async () => document.querySelector<HTMLFormElement>("#add-ticket-link")!.requestSubmit());

  expect(addLink).toHaveBeenCalledWith("ticket-1", { url: addedLink.url, title: "ENG-123" });
  expect(client.getQueryData<TicketLink[]>(qk.ticketLinks("ticket-1"))).toEqual([addedLink]);
  expect(host.textContent).toContain("ENG-123");
  expect(document.querySelector("[role='dialog']")).toBeNull();
});

it("requires an HTTPS URL before submitting", async () => {
  const addLink = vi.spyOn(api.tickets, "addLink");
  await render();
  await act(async () => [...host.querySelectorAll("button")].find((button) => button.textContent?.trim() === "Add link")!.click());
  await type(input(0), "http://linear.app/acme/issue/ENG-123");
  await act(async () => document.querySelector<HTMLFormElement>("#add-ticket-link")!.requestSubmit());

  expect(addLink).not.toHaveBeenCalled();
  expect(document.querySelector("[role='alert']")?.textContent).toBe("Enter a full HTTPS URL.");
});

it("does not show an empty Linked section to a viewer", async () => {
  await render(false);
  expect(host.textContent).toBe("");
});

it("keeps provider state visible while an agent can add another link", async () => {
  client.setQueryData<TicketLink[]>(qk.ticketLinks("ticket-1"), [
    { ...addedLink, external_state: "done", external_state_name: "Shipped" },
  ]);
  await render();
  expect(host.textContent).toContain("Shipped");
  expect([...host.querySelectorAll("button")].some((button) => button.textContent?.trim() === "Add link")).toBe(true);
});

const callRelation: TicketRelation = {
  id: "rel-1",
  ticket_id: "ticket-1",
  related: { id: "ticket-2", number: 42, subject: "Call about the invoice", status: "pending", channel: "phone" },
  actor_type: "user",
  created_by: "user-1",
  created_at: "2026-11-02T12:00:00Z",
};

const ticketRow = (id: string, number: number, subject: string) => ({ id, number, subject, status: "open", contact: null, last_message_at: "2026-11-02T12:00:00Z" }) as unknown as Ticket;

function button(label: string) {
  return [...document.querySelectorAll("button")].find((b) => b.textContent?.trim() === label)!;
}

it("shows a linked ticket with its number, subject and status, and a private one without details", async () => {
  client.setQueryData<TicketRelation[]>(qk.ticketRelations("ticket-1"), [callRelation, { ...callRelation, id: "rel-2", related: null }]);
  await render();

  const link = host.querySelector<HTMLAnchorElement>("a[href='/inbox/open/ticket-2']")!;
  expect(link.textContent).toContain("Call about the invoice");
  expect(link.textContent).toContain("#42");
  expect(link.textContent).toContain("Pending");
  expect(host.querySelector("svg[aria-label='Phone']")).not.toBeNull();
  expect(host.textContent).toContain("Private ticket");
  expect(host.querySelectorAll("a").length).toBe(1);
});

it("links another ticket from the Ticket tab of the add dialog", async () => {
  vi.spyOn(api.tickets, "list").mockResolvedValue({ data: [ticketRow("ticket-1", 7, "This ticket"), ticketRow("ticket-2", 42, "Call about the invoice")], next_cursor: null } as unknown as Page<Ticket>);
  const addRelated = vi.spyOn(api.tickets, "addRelated").mockResolvedValue(callRelation);
  await render();

  await act(async () => button("Add link").click());
  const tabs = document.querySelector("[role='dialog'] [role='radiogroup']")!;
  expect([...tabs.querySelectorAll("[role='radio']")].map((r) => r.textContent)).toEqual(["Link", "Ticket"]);
  await act(async () => [...tabs.querySelectorAll<HTMLElement>("[role='radio']")].find((r) => r.textContent === "Ticket")!.click());
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

  const dialog = document.querySelector("[role='dialog']")!;
  expect(dialog.textContent).toContain("Call about the invoice");
  expect(dialog.textContent).not.toContain("This ticket");
  await act(async () => [...dialog.querySelectorAll("button")].find((b) => b.textContent?.includes("Call about the invoice"))!.click());
  await act(async () => [...document.querySelectorAll<HTMLButtonElement>("[role='dialog'] button")].find((b) => b.textContent?.trim() === "Add link")!.click());

  expect(addRelated).toHaveBeenCalledWith("ticket-1", "ticket-2");
  expect(client.getQueryData<TicketRelation[]>(qk.ticketRelations("ticket-1"))).toEqual([callRelation]);
  expect(document.querySelector("[role='dialog']")).toBeNull();
});

it("removes a linked ticket from the panel", async () => {
  const removeRelated = vi.spyOn(api.tickets, "removeRelated").mockResolvedValue(undefined);
  client.setQueryData<TicketRelation[]>(qk.ticketRelations("ticket-1"), [callRelation]);
  await render();

  await act(async () => host.querySelector<HTMLButtonElement>("button[aria-label='Remove the link to #42']")!.click());

  expect(removeRelated).toHaveBeenCalledWith("ticket-1", "rel-1");
  expect(client.getQueryData<TicketRelation[]>(qk.ticketRelations("ticket-1"))).toEqual([]);
});
