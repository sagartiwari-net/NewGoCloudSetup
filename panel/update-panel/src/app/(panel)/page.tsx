"use client"

import Link from "next/link"
import { Bar, BarChart, CartesianGrid, XAxis } from "recharts"

import { CountUp } from "@/components/count-up"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { IpLink } from "@/components/ip-link"
import { UserLink } from "@/components/user-link"
import { useAuth } from "@/components/auth-provider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { useResource } from "@/hooks/use-resource"
import { getDashboard } from "@/lib/api/client"
import type { DashboardStats } from "@/lib/api/types"
import { formatTime } from "@/lib/format"

const chartConfig = {
  hits: { label: "Credit hits", color: "var(--chart-1)" },
} satisfies ChartConfig

type LiveUser = DashboardStats["active_users_list"][number]

const columns: DataColumn<LiveUser>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
  { id: "ip", header: "Client IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  { id: "account", header: "Assigned account", cell: (row) => row.assigned_account_name, sortValue: (row) => row.assigned_account_name },
  { id: "started", header: "Started", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
  { id: "expires", header: "Expires", cell: (row) => formatTime(row.expires_at), sortValue: (row) => row.expires_at },
]

export default function OverviewPage() {
  const { session } = useAuth()
  const { data, loading } = useResource(session?.role ?? "none", getDashboard)
  const stats = data

  return (
    <>
      <PageHeader
        title="Overview"
        description="Live sessions, mapped accounts, and credit use for the websites you can see."
      />
      <div className="grid gap-3 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardDescription>Active sessions</CardDescription>
            <CardTitle>{loading || !stats ? "—" : <CountUp value={stats.active_sessions} />}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>Mapped accounts</CardDescription>
            <CardTitle>{loading || !stats ? "—" : <CountUp value={stats.accounts_total} />}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>Credit hits today</CardDescription>
            <CardTitle>{loading || !stats ? "—" : <CountUp value={stats.credit_hits_today} />}</CardTitle>
          </CardHeader>
        </Card>
      </div>
      <div className="flex flex-col gap-3">
        <h2 className="text-lg font-medium">Needs attention</h2>
        <div className="grid gap-3 md:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle>Cookies</CardTitle>
              <CardDescription>{stats ? stats.attention.cookies.count : "—"}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              {!stats || stats.attention.cookies.items.length === 0 ? (
                <p className="text-sm text-muted-foreground">None</p>
              ) : (
                stats.attention.cookies.items.map((account) => (
                  <Link key={account.id} href="/accounts" className="text-sm underline underline-offset-4">
                    {account.name}
                  </Link>
                ))
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Limits</CardTitle>
              <CardDescription>{stats ? stats.attention.limits.count : "—"}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              {!stats || stats.attention.limits.items.length === 0 ? (
                <p className="text-sm text-muted-foreground">None</p>
              ) : (
                stats.attention.limits.items.map((row) => (
                  <p key={`${row.username}-${row.tool_name}-${row.limit}`} className="text-sm">
                    <UserLink username={row.username} /> · {row.tool_name} · {row.used} / {row.limit}
                  </p>
                ))
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Expiring</CardTitle>
              <CardDescription>{stats ? stats.attention.expiring.count : "—"}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              {!stats || stats.attention.expiring.items.length === 0 ? (
                <p className="text-sm text-muted-foreground">None</p>
              ) : (
                stats.attention.expiring.items.map((row) => (
                  <p key={`${row.username}-${row.expires_on}`} className="text-sm">
                    <UserLink username={row.username} /> · {row.tool_name} · {row.expires_on}
                  </p>
                ))
              )}
            </CardContent>
          </Card>
        </div>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Credit hits</CardTitle>
          <CardDescription>Last 7 days</CardDescription>
        </CardHeader>
        <CardContent>
          {loading || !stats ? null : (
            <ChartContainer config={chartConfig} className="h-64 w-full">
              <BarChart data={stats.credit_hits_7d}>
                <CartesianGrid vertical={false} stroke="var(--border)" />
                <XAxis dataKey="day" tickLine={false} axisLine={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar dataKey="hits" fill="var(--color-hits)" radius={4} />
              </BarChart>
            </ChartContainer>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Live users</CardTitle>
          <CardDescription>Sessions that are still inside their expiry window.</CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            rows={stats?.active_users_list ?? []}
            columns={columns}
            rowKey={(row) => `${row.username}-${row.created_at}`}
            loading={loading}
            emptyTitle="No live users"
            emptyDescription="Active logins will show up here."
          />
        </CardContent>
      </Card>
    </>
  )
}
