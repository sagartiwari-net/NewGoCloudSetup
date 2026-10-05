"use client"

import { useState } from "react"

import { AnalyticsDashboard } from "@/components/analytics/dashboard"
import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { IpLink } from "@/components/ip-link"
import { Truncated } from "@/components/truncated"
import { UserLink } from "@/components/user-link"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useResource } from "@/hooks/use-resource"
import {
  clearExtensionEvents,
  clearLogins,
  clearLogouts,
  clearSwitches,
  getAnalyticsSummary,
  listAllWebsites,
  listExtensionEvents,
  listLogins,
  listLogouts,
  listSwitches,
} from "@/lib/api/client"
import type { ExtensionEvent, LoginEvent, LogoutEvent, SwitchEvent } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { usePageIndex, usePageSize } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const loginColumns: DataColumn<LoginEvent>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "domain", header: "Website domain", cell: (row) => row.domain, sortValue: (row) => row.domain },
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
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "website", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
  { id: "from", header: "From account", cell: (row) => row.from_account_name, sortValue: (row) => row.from_account_name },
  { id: "to", header: "To account", cell: (row) => row.to_account_name, sortValue: (row) => row.to_account_name },
  {
    id: "reason",
    header: "Reason",
    cell: (row) => <Truncated value={row.reason} maxWidthClass="max-w-[220px]" />,
    sortValue: (row) => row.reason,
  },
  { id: "time", header: "Time", cell: (row) => formatTime(row.switched_at), sortValue: (row) => row.switched_at },
]

const logoutColumns: DataColumn<LogoutEvent>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "website", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
  { id: "account", header: "Logged-out account", cell: (row) => row.account_name || "—", sortValue: (row) => row.account_name },
  { id: "next", header: "Next account", cell: (row) => row.next_account_name || "—", sortValue: (row) => row.next_account_name },
  {
    id: "reason",
    header: "Reason",
    cell: (row) => <Truncated value={row.reason} maxWidthClass="max-w-[220px]" />,
    sortValue: (row) => row.reason,
  },
  { id: "ip", header: "Client IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

const extensionColumns: DataColumn<ExtensionEvent>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "domain", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
  { id: "tool", header: "Tool", cell: (row) => row.tool_key || row.tool_name, sortValue: (row) => row.tool_key },
  { id: "source", header: "Source", cell: (row) => row.source, sortValue: (row) => row.source },
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
  { id: "ip", header: "IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

export default function AnalyticsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [websiteId, setWebsiteId] = useState("all")
  const [days, setDays] = useState("7")
  const [tab, setTab] = useState("dashboard")
  const pageSize = usePageSize()
  const [loginPage, setLoginPage] = usePageIndex(pageSize)
  const [switchPage, setSwitchPage] = usePageIndex(pageSize)
  const [logoutPage, setLogoutPage] = usePageIndex(pageSize)
  const [extensionPage, setExtensionPage] = usePageIndex(pageSize)
  const [confirmClear, setConfirmClear] = useState(false)
  const [clearing, setClearing] = useState(false)

  const site = websiteId === "all" ? 0 : Number(websiteId)

  const { data: websites } = useResource(`${session?.role ?? "none"}-websites`, listAllWebsites)

  const { data: summary, loading: summaryLoading } = useResource(
    `${session?.role ?? "none"}-summary-${tab}-${site}-${days}-${query}`,
    async () => {
      if (tab !== "dashboard") return null
      return getAnalyticsSummary({ websiteId: site, days: Number(days), query })
    },
  )

  const { data: loginsData, loading: loginsLoading, reload: reloadLogins } = useResource(
    `${session?.role ?? "none"}-logins-${tab}-${pageSize}-${query}-${site}-${loginPage}`,
    async () => {
      if (tab !== "logins") return null
      return listLogins({ page: loginPage, pageSize, query, websiteId: site })
    },
  )

  const { data: switchesData, loading: switchesLoading, reload: reloadSwitches } = useResource(
    `${session?.role ?? "none"}-switches-${tab}-${pageSize}-${query}-${site}-${switchPage}`,
    async () => {
      if (tab !== "switches") return null
      return listSwitches({ page: switchPage, pageSize, query, websiteId: site })
    },
  )

  const { data: logoutsData, loading: logoutsLoading, reload: reloadLogouts } = useResource(
    `${session?.role ?? "none"}-logouts-${tab}-${pageSize}-${query}-${site}-${logoutPage}`,
    async () => {
      if (tab !== "logouts") return null
      return listLogouts({ page: logoutPage, pageSize, query, websiteId: site })
    },
  )

  const { data: extensionData, loading: extensionLoading, reload: reloadExtension } = useResource(
    `${session?.role ?? "none"}-extension-${tab}-${pageSize}-${query}-${site}-${extensionPage}`,
    async () => {
      if (tab !== "extension") return null
      return listExtensionEvents({ page: extensionPage, pageSize, query, websiteId: site })
    },
  )

  const clearMeta =
    tab === "logins"
      ? {
          title: "Clear logins",
          description: "Delete every login row? Switches, logouts, and extension activity stay.",
          run: clearLogins,
          toast: "Logins cleared",
        }
      : tab === "switches"
        ? {
            title: "Clear switches",
            description: "Delete every switch row? Logins, logouts, and extension activity stay.",
            run: clearSwitches,
            toast: "Switches cleared",
          }
        : tab === "logouts"
          ? {
              title: "Clear logouts",
              description: "Delete every logout detection row? Logins, switches, and extension activity stay.",
              run: clearLogouts,
              toast: "Logouts cleared",
            }
          : {
              title: "Clear extension activity",
              description: "Delete every extension activity row? Logins, switches, and logouts stay.",
              run: clearExtensionEvents,
              toast: "Extension activity cleared",
            }

  const resetPages = () => {
    setLoginPage(1)
    setSwitchPage(1)
    setLogoutPage(1)
    setExtensionPage(1)
  }

  return (
    <>
      <PageHeader title="Analytics" description="Dashboard overview plus logins, switches, logouts, and extension activity." />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="analytics-search">Search</FieldLabel>
          <Input
            id="analytics-search"
            value={query}
            placeholder="Username, ASIN, action, or IP"
            onChange={(event) => {
              setQuery(event.target.value)
              resetPages()
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="analytics-website">Website</FieldLabel>
          <ChoiceSelect
            id="analytics-website"
            value={websiteId}
            onValueChange={(value) => {
              setWebsiteId(value)
              resetPages()
            }}
            items={[
              { label: "All websites", value: "all" },
              ...(websites ?? []).map((website) => ({ label: website.name, value: String(website.id) })),
            ]}
          />
        </Field>
        {tab === "dashboard" ? (
          <Field className="md:w-44">
            <FieldLabel htmlFor="analytics-days">Date range</FieldLabel>
            <ChoiceSelect
              id="analytics-days"
              value={days}
              onValueChange={setDays}
              items={[
                { label: "Last 7 days", value: "7" },
                { label: "Last 30 days", value: "30" },
              ]}
            />
          </Field>
        ) : null}
      </div>
      <Tabs
        value={tab}
        onValueChange={(value) => {
          setTab(value)
        }}
      >
        <div className="flex w-full flex-wrap items-center justify-between gap-3">
          <TabsList>
            <TabsTrigger value="dashboard">Dashboard</TabsTrigger>
            <TabsTrigger value="logins">Logins</TabsTrigger>
            <TabsTrigger value="switches">Switches</TabsTrigger>
            <TabsTrigger value="logouts">Logouts</TabsTrigger>
            <TabsTrigger value="extension">Extension</TabsTrigger>
          </TabsList>
          {tab !== "dashboard" ? (
            <Button variant="outline" onClick={() => setConfirmClear(true)}>
              Clear
            </Button>
          ) : null}
        </div>
        <TabsContent value="dashboard">
          <AnalyticsDashboard summary={summary} loading={summaryLoading} websiteId={site} />
        </TabsContent>
        <TabsContent value="logins">
          <Card>
            <CardContent>
              <DataTable
                rows={loginsData?.items ?? []}
                columns={loginColumns}
                rowKey={(row) => row.id}
                loading={loginsLoading}
                paging={{
                  page: loginsData?.page ?? loginPage,
                  total: loginsData?.total ?? 0,
                  pageSize,
                  onPageChange: setLoginPage,
                }}
                resetKey={`${query}-${websiteId}`}
                emptyTitle={query ? "No matches" : "No logins"}
                emptyDescription="Search by username or client IP."
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="switches">
          <Card>
            <CardContent>
              <DataTable
                rows={switchesData?.items ?? []}
                columns={switchColumns}
                rowKey={(row) => row.id}
                loading={switchesLoading}
                paging={{
                  page: switchesData?.page ?? switchPage,
                  total: switchesData?.total ?? 0,
                  pageSize,
                  onPageChange: setSwitchPage,
                }}
                resetKey={`${query}-${websiteId}`}
                emptyTitle={query ? "No matches" : "No switches"}
                emptyDescription="Account changes show up when a session moves between logins."
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="logouts">
          <Card>
            <CardContent>
              <DataTable
                rows={logoutsData?.items ?? []}
                columns={logoutColumns}
                rowKey={(row) => row.id}
                loading={logoutsLoading}
                paging={{
                  page: logoutsData?.page ?? logoutPage,
                  total: logoutsData?.total ?? 0,
                  pageSize,
                  onPageChange: setLogoutPage,
                }}
                resetKey={`${query}-${websiteId}`}
                emptyTitle={query ? "No matches" : "No logouts"}
                emptyDescription="Cookie/session logout detection from tools appears here."
              />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="extension">
          <Card>
            <CardContent>
              <DataTable
                rows={extensionData?.items ?? []}
                columns={extensionColumns}
                rowKey={(row) => row.id}
                loading={extensionLoading}
                paging={{
                  page: extensionData?.page ?? extensionPage,
                  total: extensionData?.total ?? 0,
                  pageSize,
                  onPageChange: setExtensionPage,
                }}
                resetKey={`${query}-${websiteId}`}
                emptyTitle={query ? "No matches" : "No extension activity"}
                emptyDescription="Xray, keywords, inventory, and other tool API calls appear here."
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
      <ConfirmDialog
        open={confirmClear}
        onOpenChange={setConfirmClear}
        title={clearMeta.title}
        description={clearMeta.description}
        confirmLabel="Clear"
        pending={clearing}
        onConfirm={() => {
          setClearing(true)
          void runMutation(clearMeta.run, clearMeta.toast).then((ok) => {
            setClearing(false)
            if (!ok) return
            setConfirmClear(false)
            resetPages()
            if (tab === "logins") reloadLogins()
            if (tab === "switches") reloadSwitches()
            if (tab === "logouts") reloadLogouts()
            if (tab === "extension") reloadExtension()
          })
        }}
      />
    </>
  )
}
