"use client"

import { useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { IpLink } from "@/components/ip-link"
import { UserLink } from "@/components/user-link"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import { blockIP, listAllWebsites, listHostReports } from "@/lib/api/client"
import type { HostReport } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { runMutation } from "@/lib/mutate"
import { useListPage } from "@/lib/page-size"

const types = [
  { label: "All types", value: "all" },
  { label: "residential", value: "residential" },
  { label: "hosting", value: "hosting" },
  { label: "proxy", value: "proxy" },
  { label: "vpn", value: "vpn" },
]

export default function HostReportsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [websiteId, setWebsiteId] = useState("all")
  const [ipType, setIpType] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${websiteId}-${ipType}`,
    async () => {
      const [reports, websites] = await Promise.all([
        listHostReports({
          page,
          pageSize,
          query,
          websiteId: websiteId === "all" ? 0 : Number(websiteId),
          type: ipType === "all" ? "" : ipType,
        }),
        listAllWebsites(),
      ])
      return { reports, websites }
    },
  )
  const [pending, setPending] = useState<HostReport | null>(null)
  const [blocking, setBlocking] = useState(false)
  const rows = data?.reports.items ?? []

  const columns: DataColumn<HostReport>[] = [
    {
      id: "username",
      header: "Username",
      cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
      sortValue: (row) => row.username,
    },
    { id: "tool", header: "Tool", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
    { id: "ip", header: "IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
    { id: "location", header: "Location", cell: (row) => row.location || "—", sortValue: (row) => row.location },
    { id: "company", header: "Company", cell: (row) => row.org || "—", sortValue: (row) => row.org },
    { id: "type", header: "Type", cell: (row) => row.ip_type, sortValue: (row) => row.ip_type },
    { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
    {
      id: "block",
      header: "Block",
      cell: (row) =>
        row.blocked ? (
          <Badge variant="secondary">Blocked</Badge>
        ) : (
          <Button variant="destructive" size="sm" onClick={() => setPending(row)}>
            Block
          </Button>
        ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Host reports"
        description="Where each login came from. The tool stays open until you block that IP."
      />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="host-search">Search</FieldLabel>
          <Input
            id="host-search"
            value={query}
            placeholder="Username, IP, location, or company"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="host-website">Website</FieldLabel>
          <ChoiceSelect
            id="host-website"
            value={websiteId}
            onValueChange={(value) => {
              setWebsiteId(value)
              setPage(1)
            }}
            items={[
              { label: "All websites", value: "all" },
              ...(data?.websites ?? []).map((website) => ({ label: website.name, value: String(website.id) })),
            ]}
          />
        </Field>
        <Field className="md:w-48">
          <FieldLabel htmlFor="host-type">Type</FieldLabel>
          <ChoiceSelect
            id="host-type"
            value={ipType}
            onValueChange={(value) => {
              setIpType(value)
              setPage(1)
            }}
            items={types}
          />
        </Field>
      </div>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.reports.page ?? page,
              total: data?.reports.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${websiteId}-${ipType}`}
            emptyTitle={query || websiteId !== "all" || ipType !== "all" ? "No matches" : "No host reports"}
            emptyDescription="Reports appear when a tool records where the client connected from."
          />
        </CardContent>
      </Card>
      <ConfirmDialog
        open={Boolean(pending)}
        onOpenChange={(next) => {
          if (!next) setPending(null)
        }}
        title="Block IP"
        description={pending ? `Block ${pending.client_ip} on ${pending.website_name}? Reason: ${pending.ip_type}.` : ""}
        confirmLabel="Block"
        pending={blocking}
        onConfirm={() => {
          if (!pending) return
          setBlocking(true)
          void runMutation(
            () => blockIP({ website_id: pending.website_id, client_ip: pending.client_ip, reason: pending.ip_type }),
            "IP blocked",
          ).then((ok) => {
            setBlocking(false)
            if (!ok) return
            setPending(null)
            reload()
          })
        }}
      />
    </>
  )
}
