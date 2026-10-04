"use client"

import { Suspense } from "react"
import Link from "next/link"
import { useSearchParams } from "next/navigation"

import { useAuth } from "@/components/auth-provider"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { Truncated } from "@/components/truncated"
import { UserLink } from "@/components/user-link"
import { Badge } from "@/components/ui/badge"
import { buttonVariants } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useResource } from "@/hooks/use-resource"
import { getIpReport } from "@/lib/api/client"
import type { IpReport } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { cn } from "@/lib/utils"

type IpUser = IpReport["users"][number]
type IpLogin = IpReport["recent_logins"][number]
type IpHost = IpReport["host_reports"][number]

const userColumns: DataColumn<IpUser>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} />,
    sortValue: (row) => row.username,
  },
  { id: "logins", header: "Logins", cell: (row) => String(row.login_count), sortValue: (row) => row.login_count },
  {
    id: "websites",
    header: "Websites",
    cell: (row) => <Truncated value={row.website_names.join(", ") || "—"} />,
    sortValue: (row) => row.website_names.join(", "),
  },
  {
    id: "last",
    header: "Last seen",
    cell: (row) => (row.last_login_at ? formatTime(row.last_login_at) : "—"),
    sortValue: (row) => row.last_login_at,
  },
]

const loginColumns: DataColumn<IpLogin>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
  {
    id: "ua",
    header: "User agent",
    cell: (row) => <Truncated value={row.user_agent || "—"} />,
    sortValue: (row) => row.user_agent,
  },
  { id: "time", header: "Time", cell: (row) => formatTime(row.logged_in_at), sortValue: (row) => row.logged_in_at },
]

const hostColumns: DataColumn<IpHost>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
  {
    id: "type",
    header: "Type",
    cell: (row) => <Badge variant="secondary">{row.ip_type}</Badge>,
    sortValue: (row) => row.ip_type,
  },
  { id: "org", header: "Org", cell: (row) => <Truncated value={row.org || "—"} />, sortValue: (row) => row.org },
  {
    id: "location",
    header: "Location",
    cell: (row) => <Truncated value={row.location || "—"} />,
    sortValue: (row) => row.location,
  },
  { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

function IpReportContent() {
  const { session } = useAuth()
  const searchParams = useSearchParams()
  const ip = (searchParams.get("ip") ?? "").trim()

  const { data, loading } = useResource(`${session?.role ?? "none"}-ip-${ip}`, async () => {
    if (!ip) return null
    return getIpReport({ ip })
  })

  if (!ip) {
    return (
      <>
        <PageHeader title="IP report" description="Click any IP in the panel to open its full report." />
        <Card>
          <CardContent className="flex flex-col gap-3 py-6">
            <p className="text-sm text-muted-foreground">No IP in the URL.</p>
            <Link href="/host-reports" className={cn(buttonVariants({ variant: "outline" }), "w-fit")}>
              Back to host reports
            </Link>
          </CardContent>
        </Card>
      </>
    )
  }

  const cls = data?.classification
  const summary = data?.summary
  const lookup = cls?.lookup ?? {}

  return (
    <>
      <PageHeader
        title={ip}
        description={
          cls?.type_label
            ? `${cls.type_label}${cls.location ? ` · ${cls.location}` : ""}`
            : "IP type, location, and every user who used this address."
        }
        action={
          <Link href="/host-reports" className={buttonVariants({ variant: "outline" })}>
            Host reports
          </Link>
        }
      />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Kpi title="Type" value={cls?.type_label ?? cls?.ip_type} loading={loading} />
        <Kpi title="Users" value={summary?.user_count} loading={loading} />
        <Kpi title="Logins" value={summary?.login_count} loading={loading} />
        <Kpi title="Blocked" value={cls?.blocked ? "Yes" : "No"} loading={loading} />
      </div>

      <div className="grid gap-3 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Classification</CardTitle>
            <CardDescription>VPN / proxy / hosting (VPS-RDP like) / residential.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <Row label="Type" value={cls?.type_label || cls?.ip_type || "—"} badge />
            <Row label="Location" value={cls?.location || "—"} />
            <Row label="ISP / Org" value={cls?.org || "—"} />
            <Row label="City" value={lookup.city || "—"} />
            <Row label="Region" value={lookup.region || "—"} />
            <Row label="Country" value={lookup.country || "—"} />
            <Row label="ASN" value={lookup.as || lookup.asname || "—"} />
            <Row
              label="Flags"
              value={[
                lookup.proxy ? "proxy" : "",
                lookup.hosting ? "hosting" : "",
                lookup.mobile ? "mobile" : "",
              ]
                .filter(Boolean)
                .join(", ") || "—"}
            />
            <Row label="Classified" value={cls?.classified_at ? formatTime(cls.classified_at) : "—"} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Activity</CardTitle>
            <CardDescription>How often this IP showed up across tools.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 text-sm">
            <Row label="Distinct users" value={String(summary?.distinct_users ?? "—")} />
            <Row label="Total logins" value={String(summary?.login_count ?? "—")} />
            <Row label="First seen" value={summary?.first_seen_at ? formatTime(summary.first_seen_at) : "—"} />
            <Row label="Last seen" value={summary?.last_seen_at ? formatTime(summary.last_seen_at) : "—"} />
            <p className="pt-2 text-muted-foreground">
              Hosting usually means cloud/VPS and is often used for RDP or data-center exits. VPN/proxy flags come from
              the IP reputation lookup.
            </p>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="users">
        <TabsList>
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="logins">Recent logins</TabsTrigger>
          <TabsTrigger value="reports">Host reports</TabsTrigger>
        </TabsList>
        <TabsContent value="users">
          <Card>
            <CardContent>
              <DataTable
                rows={data?.users ?? []}
                columns={userColumns}
                rowKey={(row) => row.username}
                loading={loading}
                emptyTitle="No users"
                emptyDescription="No panel users have logged in from this IP yet."
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="logins">
          <Card>
            <CardContent>
              <DataTable
                rows={data?.recent_logins ?? []}
                columns={loginColumns}
                rowKey={(row) => row.id}
                loading={loading}
                emptyTitle="No logins"
                emptyDescription="Login history for this IP will show here."
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="reports">
          <Card>
            <CardContent>
              <DataTable
                rows={data?.host_reports ?? []}
                columns={hostColumns}
                rowKey={(row) => row.id}
                loading={loading}
                emptyTitle="No host reports"
                emptyDescription="Classification snapshots stored for this IP."
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  )
}

function Kpi({ title, value, loading }: { title: string; value?: string | number; loading: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle className="text-lg">{loading || value === undefined ? "—" : value}</CardTitle>
      </CardHeader>
    </Card>
  )
}

function Row({ label, value, badge }: { label: string; value: string; badge?: boolean }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      {badge ? <Badge variant="secondary">{value}</Badge> : <span className="text-right">{value}</span>}
    </div>
  )
}

export default function IpReportPage() {
  return (
    <Suspense
      fallback={
        <>
          <PageHeader title="IP report" description="Loading…" />
          <Card>
            <CardContent className="py-6 text-sm text-muted-foreground">Loading IP report…</CardContent>
          </Card>
        </>
      }
    >
      <IpReportContent />
    </Suspense>
  )
}
