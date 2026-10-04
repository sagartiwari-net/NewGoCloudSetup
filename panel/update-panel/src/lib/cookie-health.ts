export type CookieHealth = "fresh" | "stale" | "failed" | "never"

const DAY = 24 * 60 * 60 * 1000

export function cookieHealth(
  cookieUpdatedAt: string,
  latestIngest: "" | "saved" | "failed",
  now = Date.now(),
): CookieHealth {
  if (latestIngest === "failed") return "failed"
  if (!cookieUpdatedAt) return "never"
  const age = now - new Date(cookieUpdatedAt).getTime()
  if (age >= 2 * DAY) return "stale"
  return "fresh"
}
