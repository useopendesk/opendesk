import type { Page } from "@playwright/test";
import { expect, test } from "../lib/proof.ts";

async function openNewsletter(page: Page) {
  await page.getByRole("option", { name: /Wednesday Web Drop/ }).click();
  await expect(page.getByRole("heading", { name: "Wednesday Web Drop" })).toBeVisible();
  await page.getByRole("button", { name: "Not now" }).click();
}

test("comment on a long email without scrolling to the end", async ({ page, proof }) => {
  await proof.open("/mail");
  await openNewsletter(page);
  const field = page.getByRole("textbox", { name: "Comment, @ to mention a teammate" });
  await expect(field).toBeInViewport();
  await proof.chapter("Pinned comments", "Writing a comment and replying stay under the conversation, however long the mail is");
  await proof.shot("pinned", { caption: "A long newsletter: the comment field, the 2 comments and the replies are pinned at the bottom" });

  await field.click();
  await field.pressSequentially("Plane is in Leads now, I'll reach out on Friday.", { delay: 40 });
  await expect(page.getByRole("button", { name: "Comment", exact: true })).toBeEnabled();
  await proof.shot("writing", { caption: "Writing opens the editor in place, with who reads it" });

  await page.keyboard.press("Control+Enter");
  const posted = page.getByRole("region", { name: "Team comments" }).getByText("Plane is in Leads now");
  await expect(posted).toBeInViewport();
  await proof.dwell();
  await proof.shot("posted", { caption: "Posted: the conversation scrolls to the new comment" });
});

test("jump to the comments and reply from the pinned bar", async ({ page, proof }) => {
  await proof.open("/mail");
  await openNewsletter(page);
  await proof.chapter("Shortcuts", "Shift C comments, the count jumps to the discussion, Reply stays a click away");

  await page.keyboard.press("Shift+C");
  await expect(page.getByRole("textbox", { name: "Comment, @ to mention a teammate" })).toBeFocused();
  await proof.shot("shortcut", { caption: "Shift C starts a comment from anywhere in the conversation" });
  await page.keyboard.press("Escape");

  await page.getByRole("button", { name: "2 comments" }).click();
  await expect(page.getByRole("region", { name: "Team comments" })).toBeInViewport();
  await proof.dwell();
  await proof.shot("comments", { caption: "The count jumps to the team's discussion" });

  await page.getByRole("button", { name: "Reply", exact: true }).click();
  await expect(page.getByRole("button", { name: "Send" }).first()).toBeVisible();
  await proof.dwell();
  await proof.shot("replying", { caption: "Replying: the reply buttons step aside, the comment field stays" });
});

test.describe("dark", () => {
  test.use({ colorScheme: "dark" });

  test("the pinned comment bar in the dark theme", async ({ page, proof }) => {
    // The app is white first: dark is the member's own choice.
    await page.addInitScript(() => localStorage.setItem("od-theme", "dark"));
    await proof.open("/mail");
    await openNewsletter(page);
    await expect(page.getByRole("textbox", { name: "Comment, @ to mention a teammate" })).toBeInViewport();
    await proof.shot("pinned-dark", { caption: "The pinned bar in the dark theme" });
  });
});
