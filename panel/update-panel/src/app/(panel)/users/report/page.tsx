"use client"

import { Suspense, useMemo, useState } from "react"
import { useSearchParams } from "next/navigation"
import Link from "next/link"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { IpLink } from "@/components/ip-link"
import { StatusBadge } from "@/components/status-badge"
import { Truncated } from "@/components/truncated"
import { Badge } from "@/components/ui/badge"
import { buttonVariants } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useResource } from "@/hooks/use-resource"
import { getUserReport, listUserReportEvents } from "@/lib/api/client"
import type {
  ExtensionEvent,
  LoginEvent,
  SwitchEvent,
  UsageEvent,
  UserReportIP,
  UserReportSecurityRow,
} from "@/lib/api/types"
import { formatMeterList, formatTime } from "@/lib/format"
import { usePageIndex, usePageSize } from "@/lib/page-size"
import { cn } from "@/lib/utils"

const loginColumns: DataColumn<LoginEvent>[] = [
  { id: "domain", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
  { id: "ip", header: "Client IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  {
    id: "ua",
    header: "User agent",
    cell: (row) => <Truncated value={row.user_agent} />,
    sortValue: (row) => row.user_agent,
  },
  { id: "time", header: "Time", cell: (row) => formatTime(row.logged_in_at), sortValue: (row) => row.logged_in_at },
]

const switchColumns: DataColumn<SwitchEvent>[] = [
  { id: "domain", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
  { id: "from", header: "From", cell: (row) => row.from_account_name, sortValue: (row) => row.from_account_name },
  { id: "to", header: "To", cell: (row) => row.to_account_name, sortValue: (row) => row.to_account_name },
  { id: "reason", header: "Reason", cell: (row) => row.reason, sortValue: (row) => row.reason },
  { id: "time", header: "Time", cell: (row) => formatTime(row.switched_at), sortValue: (row) => row.switched_at },
]

const extensionColumns: DataColumn<ExtensionEvent>[] = [
  { id: "domain", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
  { id: "tool", header: "Tool", cell: (row) => row.tool_key || row.tool_name, sortValue: (row) => row.tool_key },
  { id: "action", header: "Action", cell: (row) => row.action, sortValue: (row) => row.action },
  { id: "asin", header: "ASIN", cell: (row) => row.asin || "—", sortValue: (row) => row.asin },
  { id: "market", header: "Marketplace", cell: (row) => row.marketplace || "—", sortValue: (row) => row.marketplace },
  {
    id: "query",
    header: "Query",
    cell: (row) => <Truncated value={row.query_text || "—"} />,
    sortValue: (row) => row.query_text,
  },
  {
    id: "path",
    header: "Path",
    cell: (row) => <Truncated value={row.target_path} />,
    sortValue: (row) => row.target_path,
  },
  { id: "status", header: "Status", cell: (row) => String(row.status_code || "—"), sortValue: (row) => row.status_code },
  { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

const usageColumns: DataColumn<UsageEvent>[] = [
  { id: "tool", header: "Tool", cell: (row) => row.tool_name, sortValue: (row) => row.tool_name },
  { id: "label", header: "Limit", cell: (row) => row.limit_label, sortValue: (row) => row.limit_label },
  { id: "action", header: "Action", cell: (row) => row.action, sortValue: (row) => row.action },
  {
    id: "path",
    header: "Target",
    cell: (row) => <Truncated value={row.target_path || "—"} />,
    sortValue: (row) => row.target_path,
  },
  { id: "amount", header: "Amount", cell: (row) => String(row.amount), sortValue: (row) => row.amount },
  { id: "time", header: "Time", cell: (row) => formatTime(row.timestamp), sortValue: (row) => row.timestamp },
]

const securityColumns: DataColumn<UserReportSecurityRow>[] = [
  { id: "kind", header: "Kind", cell: (row) => <Badge variant="secondary">{row.kind}</Badge>, sortValue: (row) => row.kind },
  { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
  { id: "type", header: "Type", cell: (row) => row.event_type, sortValue: (row) => row.event_type },
  { id: "ip", header: "IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  {
    id: "details",
    header: "Details",
    cell: (row) => <Truncated value={row.details || row.reason || "—"} />,
    sortValue: (row) => row.details || row.reason,
  },
  { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

const ipColumns: DataColumn<UserReportIP>[] = [
  { id: "ip", header: "IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  {
    id: "type",
    header: "Type",
    cell: (row) => <Badge variant="secondary">{row.ip_type || "unknown"}</Badge>,
    sortValue: (row) => row.ip_type,
  },
  { id: "org", header: "Org", cell: (row) => <Truncated value={row.org || "—"} />, sortValue: (row) => row.org },
  {
    id: "location",
    header: "Location",
    cell: (row) => <Truncated value={row.location || "—"} />,
    sortValue: (row) => row.location,
  },
  { id: "count", header: "Logins", cell: (row) => String(row.login_count), sortValue: (row) => row.login_count },
  {
    id: "domains",
    header: "Websites",
    cell: (row) => <Truncated value={row.domains || "—"} />,
    sortValue: (row) => row.domains,
  },
  {
    id: "blocked",
    header: "Blocked",
    cell: (row) => (row.blocked ? "Yes" : "No"),
    sortValue: (row) => (row.blocked ? 1 : 0),
  },
  {
    id: "last",
    header: "Last seen",
    cell: (row) => (row.last_seen_at ? formatTime(row.last_seen_at) : "—"),
    sortValue: (row) => row.last_seen_at,
  },
]

function UserReportContent() {
  const { session } = useAuth()
  const searchParams = useSearchParams()
  const username = (searchParams.get("username") ?? "").trim()
  // Default All websites; only honor websiteId when explicitly present and non-zero.
  const initialWebsite = (() => {
    const raw = searchParams.get("websiteId")
    if (!raw || raw === "0" || raw === "all") return "all"
    return raw
  })()
  const [websiteId, setWebsiteId] = useState(initialWebsite)
  const [tab, setTab] = useState("overview")
  const pageSize = usePageSize()
  const [loginPage, setLoginPage] = usePageIndex(pageSize)
  const [switchPage, setSwitchPage] = usePageIndex(pageSize)
  const [extensionPage, setExtensionPage] = usePageIndex(pageSize)
  const [usagePage, setUsagePage] = usePageIndex(pageSize)
  const [securityPage, setSecurityPage] = usePageIndex(pageSize)

  const site = websiteId === "all" ? 0 : Number(websiteId)

  const { data: report, loading: reportLoading } = useResource(
    `${session?.role ?? "none"}-report-${username}-${site}`,
    async () => {
      if (!username) return null
      return getUserReport({ username, websiteId: site || undefined })
    },
  )

  const websiteItems = useMemo(() => {
    const rows = report?.websites ?? []
    return [
      { label: "All websites", value: "all" },
      ...rows.map((website) => ({ label: `${website.name} (${website.tool_name})`, value: String(website.id) })),
    ]
  }, [report?.websites])

  const { data: logins, loading: loginsLoading } = useResource(
    `${session?.role ?? "none"}-ur-logins-${tab}-${username}-${site}-${loginPage}-${pageSize}`,
    async () => {
      if (!username || tab !== "logins") return null
      return listUserReportEvents({ section: "logins", username, websiteId: site || undefined, page: loginPage, pageSize })
    },
  )

  const { data: switches, loading: switchesLoading } = useResource(
    `${session?.role ?? "none"}-ur-switches-${tab}-${username}-${site}-${switchPage}-${pageSize}`,
    async () => {
      if (!username || tab !== "switches") return null
      return listUserReportEvents({
        section: "switches",
        username,
        websiteId: site || undefined,
        page: switchPage,
        pageSize,
      })
    },
  )

  const { data: extension, loading: extensionLoading } = useResource(
    `${session?.role ?? "none"}-ur-extension-${tab}-${username}-${site}-${extensionPage}-${pageSize}`,
    async () => {
      if (!username || tab !== "extension") return null
      return listUserReportEvents({
        section: "extension",
        username,
        websiteId: site || undefined,
        page: extensionPage,
        pageSize,
      })
    },
  )

  const { data: usage, loading: usageLoading } = useResource(
    `${session?.role ?? "none"}-ur-usage-${tab}-${username}-${site}-${usagePage}-${pageSize}`,
    async () => {
      if (!username || tab !== "quota") return null
      return listUserReportEvents({ section: "usage", username, websiteId: site || undefined, page: usagePage, pageSize })
    },
  )

  const { data: security, loading: securityLoading } = useResource(
    `${session?.role ?? "none"}-ur-security-${tab}-${username}-${site}-${securityPage}-${pageSize}`,
    async () => {
      if (!username || tab !== "security") return null
      return listUserReportEvents({
        section: "security",
        username,
        websiteId: site || undefined,
        page: securityPage,
        pageSize,
      })
    },
  )

  if (!username) {
    return (
      <>
        <PageHeader title="User report" description="Pick a username from any table to open their report." />
        <Card>
          <CardContent className="flex flex-col gap-3 py-6">
            <p className="text-sm text-muted-foreground">No username in the URL.</p>
            <Link href="/users" className={cn(buttonVariants({ variant: "outline" }), "w-fit")}>
              Back to users
            </Link>
          </CardContent>
        </Card>
      </>
    )
  }

  const summary = report?.summary
  const meters = report?.meters?.length ? report.meters : report?.user?.meters ?? []

  return (
    <>
      <PageHeader
        title={username}
        description={
          summary?.last_login_at
            ? `Last login ${formatTime(summary.last_login_at)}`
            : "Structured history across logins, limits, extension, and security."
        }
        action={
          <Link href="/users" className={buttonVariants({ variant: "outline" })}>
            Back to users
          </Link>
        }
      />

      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:w-72">
          <FieldLabel htmlFor="report-website">Website</FieldLabel>
          <ChoiceSelect
            id="report-website"
            value={websiteId}
            onValueChange={(value) => {
              setWebsiteId(value)
              setLoginPage(1)
              setSwitchPage(1)
              setExtensionPage(1)
              setUsagePage(1)
              setSecurityPage(1)
            }}
            items={websiteItems}
          />
        </Field>
        {report?.user ? (
          <div className="flex items-center gap-2 pb-1">
            <StatusBadge status={report.user.status} />
            <span className="text-sm text-muted-foreground">
              {report.user.tool_name} · {report.user.website_name}
            </span>
          </div>
        ) : null}
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <Kpi title="Logins" value={summary?.login_count} loading={reportLoading} />
        <Kpi title="Distinct IPs" value={summary?.distinct_ips} loading={reportLoading} />
        <Kpi title="Switches" value={summary?.switch_count} loading={reportLoading} />
        <Kpi title="Extension" value={summary?.extension_count} loading={reportLoading} />
        <Kpi title="Usage hits" value={summary?.usage_hit_count} loading={reportLoading} />
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="ips">IPs</TabsTrigger>
          <TabsTrigger value="logins">Logins</TabsTrigger>
          <TabsTrigger value="switches">Switches</TabsTrigger>
          <TabsTrigger value="quota">Quota / Limits</TabsTrigger>
          <TabsTrigger value="extension">Extension</TabsTrigger>
          <TabsTrigger value="security">Security</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <div className="grid gap-3 xl:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Websites / tools</CardTitle>
                <CardDescription>Where this username has activity.</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-2">
                {(report?.websites ?? []).length === 0 && !reportLoading ? (
                  <p className="text-sm text-muted-foreground">No websites found for this user.</p>
                ) : null}
                {(report?.websites ?? []).map((website) => (
                  <div key={website.id} className="flex items-center justify-between gap-3 text-sm">
                    <span>
                      {website.name}
                      <span className="text-muted-foreground"> · {website.tool_name}</span>
                    </span>
                    <span className="text-muted-foreground">{website.domain}</span>
                  </div>
                ))}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Session & limits</CardTitle>
                <CardDescription>Current session and meter snapshot.</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-3 text-sm">
                {summary?.active_session ? (
                  <div className="flex flex-col gap-1">
                    <span className="flex flex-wrap items-center gap-1">
                      Active on {summary.active_session.website_name} · IP{" "}
                      <IpLink ip={summary.active_session.client_ip} />
                    </span>
                    <span className="text-muted-foreground">
                      Account {summary.active_session.assigned_account_name} · expires{" "}
                      {formatTime(summary.active_session.expires_at)}
                    </span>
                  </div>
                ) : (
                  <p className="text-muted-foreground">No active session.</p>
                )}
                <div>
                  <p className="mb-1 font-medium">Meters</p>
                  <p className="text-muted-foreground">{meters.length ? formatMeterList(meters) : "No meters"}</p>
                </div>
              </CardContent>
            </Card>
            <Card className="xl:col-span-2">
              <CardHeader>
                <CardTitle>Recent IPs</CardTitle>
                <CardDescription>Where this account opened, with IP type when known.</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-2">
                {(report?.ips ?? []).length === 0 && !reportLoading ? (
                  <p className="text-sm text-muted-foreground">No login IPs yet.</p>
                ) : null}
                {(report?.ips ?? []).slice(0, 8).map((row) => (
                  <div key={row.client_ip} className="flex flex-wrap items-center justify-between gap-2 text-sm">
                    <IpLink ip={row.client_ip} className="font-medium" />
                    <Badge variant="secondary">{row.ip_type || "unknown"}</Badge>
                    <span className="text-muted-foreground">
                      {row.login_count} logins
                      {row.org ? ` · ${row.org}` : ""}
                      {row.location ? ` · ${row.location}` : ""}
                    </span>
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="ips">
          <Card>
            <CardContent>
              <DataTable
                rows={report?.ips ?? []}
                columns={ipColumns}
                rowKey={(row) => row.client_ip}
                loading={reportLoading}
                emptyTitle="No IPs"
                emptyDescription="Login and host-report IPs for this username show here with type, org, and location."
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="logins">
          <Card>
            <CardContent>
              <DataTable
                rows={(logins?.items as LoginEvent[] | undefined) ?? []}
                columns={loginColumns}
                rowKey={(row) => row.id}
                loading={loginsLoading}
                paging={{
                  page: logins?.page ?? loginPage,
                  total: logins?.total ?? 0,
                  pageSize,
                  onPageChange: setLoginPage,
                }}
                resetKey={`${username}-${websiteId}`}
                emptyTitle="No logins"
                emptyDescription="Login rows for this username will show here."
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="switches">
          <Card>
            <CardContent>
              <DataTable
                rows={(switches?.items as SwitchEvent[] | undefined) ?? []}
                columns={switchColumns}
                rowKey={(row) => row.id}
                loading={switchesLoading}
                paging={{
                  page: switches?.page ?? switchPage,
                  total: switches?.total ?? 0,
                  pageSize,
                  onPageChange: setSwitchPage,
                }}
                resetKey={`${username}-${websiteId}`}
                emptyTitle="No switches"
                emptyDescription="Account switch history for this username."
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="quota">
          <Card className="mb-3">
            <CardHeader>
              <CardTitle>Current meters</CardTitle>
              <CardDescription>{meters.length ? formatMeterList(meters) : "No panel user meters."}</CardDescription>
            </CardHeader>
          </Card>
          <Card>
            <CardContent>
              <DataTable
                rows={(usage?.items as UsageEvent[] | undefined) ?? []}
                columns={usageColumns}
                rowKey={(row) => row.id}
                loading={usageLoading}
                paging={{
                  page: usage?.page ?? usagePage,
                  total: usage?.total ?? 0,
                  pageSize,
                  onPageChange: setUsagePage,
                }}
                resetKey={`${username}-${websiteId}`}
                emptyTitle="No usage"
                emptyDescription="Quota / search hit logs for this username."
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="extension">
          <Card>
            <CardContent>
              <DataTable
                rows={(extension?.items as ExtensionEvent[] | undefined) ?? []}
                columns={extensionColumns}
                rowKey={(row) => row.id}
                loading={extensionLoading}
                paging={{
                  page: extension?.page ?? extensionPage,
                  total: extension?.total ?? 0,
                  pageSize,
                  onPageChange: setExtensionPage,
                }}
                resetKey={`${username}-${websiteId}`}
                emptyTitle="No extension activity"
                emptyDescription="Xray and other extension API calls for this username."
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="security">
          <Card>
            <CardContent>
              <DataTable
                rows={(security?.items as UserReportSecurityRow[] | undefined) ?? []}
                columns={securityColumns}
                rowKey={(row) => `${row.kind}-${row.id}`}
                loading={securityLoading}
                paging={{
                  page: security?.page ?? securityPage,
                  total: security?.total ?? 0,
                  pageSize,
                  onPageChange: setSecurityPage,
                }}
                resetKey={`${username}-${websiteId}`}
                emptyTitle="No security events"
                emptyDescription="Violations and security events for this username."
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </>
  )
}

function Kpi({ title, value, loading }: { title: string; value?: number; loading: boolean }) {
  return (
    <Card>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle>{loading || value === undefined ? "—" : value}</CardTitle>
      </CardHeader>
    </Card>
  )
}

export default function UserReportPage() {
  return (
    <Suspense
      fallback={
        <>
          <PageHeader title="User report" description="Loading…" />
          <Card>
            <CardContent className="py-6 text-sm text-muted-foreground">Loading report…</CardContent>
          </Card>
        </>
      }
    >
      <UserReportContent />
    </Suspense>
  )
}
