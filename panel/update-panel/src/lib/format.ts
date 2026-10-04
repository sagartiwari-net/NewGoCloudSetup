export function maskSecret(value: string) {
  if (!value) return "—"
  return `••••${value.slice(-4)}`
}

export function maskEndpoint(value: string) {
  if (!value) return "—"
  if (!value.includes("@")) return value
  return `••••${value.slice(-4)}`
}

export function formatTime(value: string) {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date)
}

export function dayKey(value: string | Date) {
  const date = typeof value === "string" ? new Date(value) : value
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

export function formatReset(days: number) {
  return days === 1 ? "every 1 day" : `every ${days} days`
}

export function formatLimit(limit: number) {
  if (limit < 0) return "Unlimited"
  if (limit === 0) return "Blocked"
  return String(limit)
}

export function limitHint(value: number) {
  if (value < 0) return "-1 is unlimited. The counter still runs, and the action is not stopped."
  if (value === 0) return "0 blocks this action."
  return "A positive number is the cap for the reset period. -1 is unlimited. 0 blocks this action."
}

export function formatMeter(meter: { label: string; used: number; limit: number }) {
  return `${meter.label} ${meter.used} / ${formatLimit(meter.limit)}`
}

export function formatMeterList(meters: Array<{ label: string; used: number; limit: number }>) {
  return meters.map((meter) => formatMeter(meter)).join(" · ")
}

export function logType(event: { limit_key: string; limit_label: string; reset_days: number }) {
  if (event.reset_days === 1 && event.limit_key === "credits") return "Daily Credit Hit"
  return event.limit_label
}

export function formatIst(value: string) {
  const date = new Date(value)
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: "Asia/Kolkata",
    day: "numeric",
    month: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
    hour12: true,
  }).formatToParts(date)
  const pick = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? ""
  const dayPeriod = pick("dayPeriod").toLowerCase()
  return `${pick("day")}/${pick("month")}/${pick("year")}, ${pick("hour")}:${pick("minute")}:${pick("second")} ${dayPeriod}`
}

export function dayLabel(key: string) {
  const date = new Date(`${key}T12:00:00`)
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(date)
}
