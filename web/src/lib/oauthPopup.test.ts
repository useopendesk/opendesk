import { afterEach, describe, expect, it, vi } from "vitest";
import { deliverPopupResult, newPopupId, waitForPopup, type PopupResult } from "./oauthPopup";

const result = (popup: string, extra: Partial<PopupResult> = {}): PopupResult => ({ type: "opendesk-oauth-popup", popup, code: "one-time", ...extra });

afterEach(() => vi.useRealTimers());

describe("popup sign-in", () => {
  it("makes ids the API accepts", () => {
    const ids = new Set(Array.from({ length: 50 }, newPopupId));
    expect(ids.size).toBe(50);
    for (const id of ids) expect(id).toMatch(/^[A-Za-z0-9_-]{16,64}$/);
  });

  it("hands the code to the waiting tab over the channel and is acknowledged", async () => {
    const id = newPopupId();
    const got = vi.fn();
    const stop = waitForPopup(id, got);
    await expect(deliverPopupResult(result(id))).resolves.toBe(true);
    expect(got).toHaveBeenCalledTimes(1);
    expect(got.mock.calls[0][0].code).toBe("one-time");
    stop();
  });

  it("ignores another sign-in's result, so the popup finishes on its own", async () => {
    const got = vi.fn();
    const stop = waitForPopup(newPopupId(), got);
    await expect(deliverPopupResult(result(newPopupId()))).resolves.toBe(false);
    expect(got).not.toHaveBeenCalled();
    stop();
  });

  it("takes a result from a window message of this origin only, once", () => {
    const id = newPopupId();
    const got = vi.fn();
    const stop = waitForPopup(id, got);
    window.dispatchEvent(new MessageEvent("message", { data: result(id), origin: "https://evil.example" }));
    expect(got).not.toHaveBeenCalled();
    window.dispatchEvent(new MessageEvent("message", { data: result(id, { code: undefined, error: "access_denied" }), origin: window.location.origin }));
    window.dispatchEvent(new MessageEvent("message", { data: result(id), origin: window.location.origin }));
    expect(got).toHaveBeenCalledTimes(1);
    expect(got.mock.calls[0][0].error).toBe("access_denied");
    stop();
  });

  it("stops listening once cancelled", async () => {
    const id = newPopupId();
    const got = vi.fn();
    waitForPopup(id, got)();
    await expect(deliverPopupResult(result(id))).resolves.toBe(false);
    expect(got).not.toHaveBeenCalled();
  });
});
