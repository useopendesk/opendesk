import type { Page } from "@playwright/test";
import { expect, test } from "../lib/proof.ts";

// A Collab account's addresses (contracts/collab.md "Addresses"): the mock's Gmail account
// "Work" sends as support@, billing@ (each with its own designed signature) and press@
// (still being confirmed in Gmail), and a customer wrote to support@. Gmail lets apps change
// only the main address's signature, so an alias's is kept in opendesk.

async function quietShell(page: Page) {
  // The ask for notification permission, put away: it is not what these flows are about.
  await page.addInitScript(() => localStorage.setItem("od-notify-ask", String(Date.now())));
}

test("an account's addresses, each with its own signature", async ({ page, proof }) => {
  await quietShell(page);
  await proof.open("/settings/mail-accounts");
  // The Gmail account's row (a button): its name and address.
  await page.getByRole("button", { name: /^Work Private/ }).click();
  const addresses = page.getByRole("list", { name: "Addresses" });
  await addresses.scrollIntoViewIfNeeded();
  await expect(addresses.getByText("support@margin.app")).toBeVisible();
  await expect(addresses.getByText("Waiting for confirmation")).toBeVisible();
  await proof.chapter("Addresses", "Every address Gmail lets this account send as, with its own From name and signature");
  await proof.shot("addresses", { caption: "Gmail's send-as addresses: support@ and billing@ can send, press@ waits for confirmation in Gmail" });

  const support = addresses.getByRole("listitem").filter({ hasText: "support@margin.app" });
  await expect(support.getByText("Kept in opendesk, follows changes in Gmail")).toBeVisible();
  await support.getByRole("button", { name: "Edit" }).click();
  const drawer = page.getByRole("dialog", { name: "Edit support@margin.app" });
  await expect(drawer.getByText("Gmail only lets other apps change the main address's signature", { exact: false })).toBeVisible();
  // support@'s signature is a designed one: it opens as HTML with its preview.
  const preview = drawer.getByRole("region", { name: "Preview" });
  await expect(preview.getByText("We usually answer within a few hours.")).toBeVisible();
  await expect(drawer.getByRole("button", { name: "Save" })).toBeDisabled();
  await proof.shot("edit-address", { caption: "support@'s own signature, drawn as sent; Gmail's changes come in, edits here stay in opendesk" });

  const source = drawer.getByRole("textbox", { name: "Signature (HTML)" });
  await source.click();
  await source.press("ControlOrMeta+End");
  await source.pressSequentially('<div style="color:#6b7280">Status page: status.margin.app</div>', { delay: 15 });
  await expect(preview.getByText("Status page: status.margin.app")).toBeVisible();
  await proof.dwell();
  await proof.shot("address-edited", { caption: "An edit shows in the preview at once" });
  await drawer.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Saved support@margin.app.")).toBeVisible();
  await expect(drawer).toBeHidden();
  await proof.dwell();
  await proof.shot("address-saved", { caption: "Saved in opendesk, with no error from Gmail: this signature goes out from support@" });

  // Get from Gmail puts Gmail's signature back.
  await support.getByRole("button", { name: "Edit" }).click();
  await expect(preview.getByText("Status page: status.margin.app")).toBeVisible();
  await drawer.getByRole("button", { name: "Get from Gmail" }).click();
  await expect(page.getByText("Signature updated from Gmail")).toBeVisible();
  await expect(preview.getByText("Status page: status.margin.app")).toHaveCount(0);
  await expect(preview.getByText("We usually answer within a few hours.")).toBeVisible();
  await proof.dwell();
  await proof.shot("address-pulled", { caption: "Get from Gmail: support@'s signature as Gmail has it now" });
});

test("reply from the alias a customer wrote to, with its signature", async ({ page, proof }) => {
  await quietShell(page);
  await proof.open("/mail");
  const row = page.getByRole("option", { name: /Export to CSV keeps timing out/ });
  await expect(row.getByText("support@margin.app")).toBeVisible();
  await proof.chapter("Written to an alias", "The list says which of your addresses a conversation was written to");
  await proof.shot("list-alias", { caption: "Written to support@: the address shows beside the subject" });

  await row.click();
  await expect(page.getByRole("heading", { name: "Export to CSV keeps timing out" })).toBeVisible();
  await expect(page.getByRole("button", { name: "To support@margin.app" })).toBeVisible();
  await page.getByRole("button", { name: "Reply", exact: true }).click();
  const from = page.getByRole("combobox", { name: "From" });
  await expect(from).toContainText("Margin Support <support@margin.app>");
  await expect(page.getByText("We usually answer within a few hours.")).toBeVisible();
  await proof.chapter("Reply from it", "A reply goes out from support@ with support@'s signature");
  await proof.shot("reply-from-alias", { caption: "Replying: From is support@, and its signature is in the message" });

  await from.click();
  const listbox = page.getByRole("listbox");
  await expect(listbox.getByRole("option", { name: /billing@margin\.app/ })).toBeVisible();
  await expect(listbox.getByRole("option", { name: /press@margin\.app/ })).toHaveCount(0);
  await proof.shot("from-picker", { caption: "From lists the account's addresses that can send; press@ is not confirmed yet" });

  await listbox.getByRole("option", { name: /billing@margin\.app/ }).click();
  await expect(page.getByText("Invoices and receipts")).toBeVisible();
  await expect(page.getByText("We usually answer within a few hours.")).toHaveCount(0);
  await proof.dwell();
  await proof.shot("signature-follows", { caption: "Another address: its signature takes the place of support@'s" });
});
