const DAY = 24 * 60 * 60 * 1000

export function isOlderThan(value: string, days: number, now = Date.now()) {
  const time = new Date(value).getTime()
  if (Number.isNaN(time)) return false
  return now - time >= days * DAY
}

export function keepUsage<T extends { timestamp: string; reset_days: number }>(rows: T[], now = Date.now()) {
  return rows.filter((row) => !isOlderThan(row.timestamp, row.reset_days, now))
}

export function keepWeek<T extends { created_at: string }>(rows: T[], now = Date.now()) {
  return rows.filter((row) => !isOlderThan(row.created_at, 7, now))
}
