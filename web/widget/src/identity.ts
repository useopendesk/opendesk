// The signed-in user's user_token (contracts/teams.md "Widget identity"): a JWT the host
// page's server signs with the widget secret. The widget never verifies it, the API does; it
// only reads `exp` to renew the token before it runs out.

/** Renew this long before `exp`, so a request never carries a token about to expire. */
export const RENEW_BEFORE_MS = 60_000;

/** The token's `exp` in ms, read without verifying; null when it has none or is not a JWT. */
export function tokenExpiry(token: string): number | null {
  try {
    const part = token.split(".")[1];
    if (!part) return null;
    const b64 = part.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(part.length / 4) * 4, "=");
    const exp = (JSON.parse(atob(b64)) as Record<string, unknown>).exp;
    return typeof exp === "number" && Number.isFinite(exp) ? exp * 1000 : null;
  } catch {
    return null;
  }
}

/** The token has expired, or will within RENEW_BEFORE_MS of `now`. */
export function expiresSoon(token: string, now: number): boolean {
  const exp = tokenExpiry(token);
  return exp !== null && exp - now <= RENEW_BEFORE_MS;
}
