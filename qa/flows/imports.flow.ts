import { expect, test } from "../lib/proof.ts";

test("choose Front as the source for a helpdesk import", async ({ page, proof }) => {
  await page.addInitScript(() => localStorage.setItem("od-ollie-tips", JSON.stringify({ paused: Number.MAX_SAFE_INTEGER })));
  await proof.open("/settings/import");
  const front = page.getByRole("button", { name: /Front API token/ });
  await expect(page.getByRole("heading", { name: "Where is your history now?" })).toBeVisible();
  await expect(front).toBeVisible();
  await proof.chapter("Import from Front", "Choose Front and see the read-only access it needs");
  await proof.shot("sources", { caption: "Front is available alongside the other import sources" });

  await front.click();
  const token = page.getByLabel("API token");
  await expect(page.getByRole("heading", { name: "Import from Front" })).toBeVisible();
  await expect(token).toBeVisible();
  await expect(page.getByText(/Create a read-only token/)).toBeVisible();
  await proof.dwell();
  await proof.shot("front-token", { caption: "Front asks for a read-only token with shared inbox access" });
});
