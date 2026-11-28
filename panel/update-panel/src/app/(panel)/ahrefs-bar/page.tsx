"use client"

import Link from "next/link"
import { useAuth } from "@/components/auth-provider"
import { PageHeader } from "@/components/page-header"
import { Card, CardContent } from "@/components/ui/card"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { getAhrefsBarOverview } from "@/lib/api/client"

const links = [
  { href: "/ahrefs-bar/accounts", label: "Accounts", hint: "Ahrefs token pool" },
  { href: "/ahrefs-bar/portals", label: "Portals", hint: "aMember / tm-check sites" },
  { href: "/ahrefs-bar/defaults", label: "Defaults", hint: "Hourly / daily / TTL" },
  { href: "/ahrefs-bar/users", label: "User limits", hint: "Per-login overrides" },
  { href: "/ahrefs-bar/usage", label: "Usage", hint: "Request log" },
  { href: "/ahrefs-bar/download", label: "Download", hint: "Extension zip + tm-check" },
]

export default function AhrefsBarOverviewPage() {
  const { session } = useAuth()
  const { data, loading } = useResource(`${session?.role ?? "none"}-overview`, () => getAhrefsBarOverview())

  const stats = [
    { label: "Active accounts", value: data?.accounts_active ?? 0 },
    { label: "Bad / cooldown", value: data?.accounts_bad ?? 0 },
    { label: "Portals", value: data?.portals_total ?? 0 },
    { label: "Credits (24h)", value: data?.credits_24h ?? 0 },
    { label: "Requests (24h)", value: data?.requests_24h ?? 0 },
    { label: "Cache hits (24h)", value: data?.cache_hits_24h ?? 0 },
    { label: "Errors (24h)", value: data?.errors_24h ?? 0 },
    { label: "Accounts total", value: data?.accounts_total ?? 0 },
  ]

  return (
    <>
      <PageHeader
        title="Ahrefs Bar"
        description="Manage token pools, portals, credit defaults, and usage for the ToolsMandi Ahrefs toolbar."
      />
      {loading && !data ? (
        <div className="flex justify-center py-16"><Spinner /></div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {stats.map((s) => (
            <Card key={s.label}>
              <CardContent className="pt-6">
                <div className="text-sm text-muted-foreground">{s.label}</div>
                <div className="mt-1 text-2xl font-semibold tabular-nums">{s.value}</div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      <div className="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {links.map((l) => (
          <Link key={l.href} href={l.href} className="rounded-xl border bg-card p-4 transition hover:border-foreground/20">
            <div className="font-medium">{l.label}</div>
            <div className="mt-1 text-sm text-muted-foreground">{l.hint}</div>
          </Link>
        ))}
      </div>
    </>
  )
}
