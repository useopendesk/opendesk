import { describe, expect, it } from "vitest";
import { offersNoTls } from "./mailHost";

describe("offersNoTls", () => {
  it("offers no encryption only for local and private hosts", () => {
    for (const h of ["localhost", "mailpit", "mail.local", "127.0.0.1", "10.0.0.5", "172.20.1.1", "192.168.1.10", "::1", "[fd00::1]"]) expect(offersNoTls(h), h).toBe(true);
    for (const h of ["", "smtp.office365.com", "imap.gmail.com", "fcm.example.com", "8.8.8.8", "172.32.0.1"]) expect(offersNoTls(h), h).toBe(false);
  });
});
