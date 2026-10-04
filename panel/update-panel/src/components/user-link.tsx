"use client"

import Link from "next/link"

import { cn } from "@/lib/utils"

export function userReportHref(username: string, websiteId?: number) {
  const params = new URLSearchParams({ username })
  if (websiteId && websiteId > 0) params.set("websiteId", String(websiteId))
  return `/users/report?${params.toString()}`
}

export function UserLink({
  username,
  websiteId,
  className,
}: {
  username: string
  websiteId?: number
  className?: string
}) {
  if (!username) return <span className={className}>—</span>
  return (
    <Link
      href={userReportHref(username, websiteId)}
      className={cn("text-primary underline underline-offset-4 hover:opacity-90", className)}
      onClick={(event) => event.stopPropagation()}
    >
      {username}
    </Link>
  )
}
