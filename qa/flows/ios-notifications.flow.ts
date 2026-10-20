import { test } from "../lib/ios.ts";

test.use({ start: "/notifications" });

test("a new unassigned ticket in the iPhone notifications", async ({ ios }) => {
  await ios.shot("notifications", { caption: "New ticket from Elena Rossi, among the other notifications" });
  await ios.open("/inbox/unassigned/1071");
  await ios.shot("ticket", { caption: "The new ticket, still unassigned" });
});
