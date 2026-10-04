"use client"

import Link from "next/link"

import { cn } from "@/lib/utils"

export function ipReportHref(ip: string) {
  const value = ip.trim()
  if (!value) return "/ips/report"
  return `/ips/report?ip=${encodeURIComponent(value)}`
}

export function IpLink({
  ip,
  className,
}: {
  ip: string
  className?: string
}) {
  const value = (ip ?? "").trim()
  if (!value || value === "—") return <span className={className}>—</span>
  return (
    <Link
      href={ipReportHref(value)}
      className={cn("font-mono text-primary underline underline-offset-4 hover:opacity-90", className)}
      onClick={(event) => event.stopPropagation()}
    >
      {value}
    </Link>
  )
}
