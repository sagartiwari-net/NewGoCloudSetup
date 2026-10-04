"use client"

import { useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { Truncated } from "@/components/truncated"
import { IpLink } from "@/components/ip-link"
import { UserLink } from "@/components/user-link"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import { listAllWebsites, listSecurity } from "@/lib/api/client"
import type { SecurityEvent } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { useListPage } from "@/lib/page-size"

const eventTypes = [
  { label: "Cookie sharing", value: "cookie_share" },
  { label: "Access pattern", value: "access_pattern" },
  { label: "path_blocked", value: "path_blocked" },
  { label: "ip_blocked", value: "ip_blocked" },
  { label: "session_mismatch", value: "session_mismatch" },
  { label: "rate_limited", value: "rate_limited" },
]

const columns: DataColumn<SecurityEvent>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
  { id: "ip", header: "Client IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  { id: "type", header: "Event type", cell: (row) => <Badge variant="secondary">{row.event_type}</Badge>, sortValue: (row) => row.event_type },
  { id: "url", header: "Attempted URL", cell: (row) => <Truncated value={row.attempted_url} />, sortValue: (row) => row.attempted_url },
  { id: "details", header: "Details", cell: (row) => row.details, sortValue: (row) => row.details },
  { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

export default function SecurityPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [eventType, setEventType] = useState("all")
  const [websiteId, setWebsiteId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${eventType}-${websiteId}`,
    async () => {
      const [events, websites] = await Promise.all([
        listSecurity({
          page,
          pageSize,
          query,
          websiteId: websiteId === "all" ? 0 : Number(websiteId),
          eventType: eventType === "all" ? "" : eventType,
        }),
        listAllWebsites(),
      ])
      return { events, websites }
    },
  )
  const rows = data?.events.items ?? []

  return (
    <>
      <PageHeader title="Security" description="Cookie sharing, repeated access, and other events. Kept for 7 days." />
      <div className="flex flex-col gap-3 md:flex-row md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="security-search">Search</FieldLabel>
          <Input
            id="security-search"
            value={query}
            placeholder="Username, IP, or URL"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="security-type">Event type</FieldLabel>
          <ChoiceSelect
            id="security-type"
            value={eventType}
            onValueChange={(value) => {
              setEventType(value)
              setPage(1)
            }}
            items={[
              { label: "All event types", value: "all" },
              ...eventTypes,
            ]}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="security-website">Website</FieldLabel>
          <ChoiceSelect
            id="security-website"
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
      </div>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.events.page ?? page,
              total: data?.events.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${eventType}-${websiteId}`}
            emptyTitle={query || eventType !== "all" ? "No matches" : "No security events"}
            emptyDescription="Search by username, IP, or attempted URL."
          />
        </CardContent>
      </Card>
    </>
  )
}
