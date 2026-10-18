import { describe, expect, it } from "vitest";
import { dayLabel, duration, humanize, number, plural, relative, shortDate, usd } from "./format";

describe("usd", () => {
  it("shows cents for ordinary amounts", () => {
    expect(usd(1234.56)).toBe("$1,234.56");
    expect(usd(10)).toBe("$10.00");
    expect(usd(0)).toBe("$0.00");
    expect(usd(0.5)).toBe("$0.50");
    expect(usd(-3.2)).toBe("-$3.20");
  });

  it("keeps up to four decimals under a cent", () => {
    expect(usd(0.0042)).toBe("$0.0042");
    expect(usd(0.00425)).toBe("$0.0043");
    expect(usd(0.005)).toBe("$0.005");
    expect(usd(0.00005)).toBe("<$0.0001");
  });

  it("shows a dash for missing values", () => {
    expect(usd(null)).toBe("–");
    expect(usd(Number.NaN)).toBe("–");
  });
});

describe("dates", () => {
  const now = Date.parse("2026-10-18T12:00:00Z");
  const ago = (s: number) => new Date(now - s * 1000).toISOString();

  it("are relative", () => {
    expect(relative(ago(10), now)).toBe("just now");
    expect(relative(ago(60), now)).toBe("1 minute ago");
    expect(relative(ago(5 * 60), now)).toBe("5 minutes ago");
    expect(relative(ago(3 * 3600), now)).toBe("3 hours ago");
    expect(relative(ago(2 * 86400), now)).toBe("2 days ago");
    expect(relative(new Date(now + 2 * 3600_000).toISOString(), now)).toBe("in 2 hours");
    expect(relative(null, now)).toBe("");
  });

  it("fall back to a date after a month", () => {
    expect(relative(ago(60 * 86400), now)).toBe(shortDate(ago(60 * 86400), now));
    expect(shortDate("2024-03-04T10:00:00Z", now)).toBe("Mar 4, 2024");
  });

  it("label UTC days for charts", () => {
    expect(dayLabel("2026-10-04")).toBe("Oct 4");
    expect(dayLabel("bad")).toBe("bad");
  });
});

describe("numbers", () => {
  it("group and pluralize", () => {
    expect(number(12345)).toBe("12,345");
    expect(plural(1, "member")).toBe("1 member");
    expect(plural(1200, "member")).toBe("1,200 members");
  });

  it("read durations", () => {
    expect(duration(42)).toBe("42s");
    expect(duration(135)).toBe("2m 15s");
    expect(duration(7200)).toBe("2h");
    expect(duration(90000)).toBe("1d 1h");
    expect(duration(null)).toBe("–");
  });

  it("humanize codes", () => {
    expect(humanize("workspace.limits_changed")).toBe("Workspace limits changed");
    expect(humanize("self_hosted")).toBe("Self hosted");
  });
});
