"use client"

import { useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { IpLink } from "@/components/ip-link"
import { UserLink } from "@/components/user-link"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import { listAllWebsites, listResellers, listViolations, type Reseller } from "@/lib/api/client"
import type { Violation } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { useListPage } from "@/lib/page-size"

const columns: DataColumn<Violation>[] = [
  {
    id: "username",
    header: "Username",
    cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
    sortValue: (row) => row.username,
  },
  { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
  { id: "ip", header: "Client IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  { id: "reason", header: "Reason", cell: (row) => row.reason, sortValue: (row) => row.reason },
  { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

export default function ViolationsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [websiteId, setWebsiteId] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${websiteId}-${resellerId}`,
    async () => {
      const [violations, websites, resellers] = await Promise.all([
        listViolations({
          page,
          pageSize,
          query,
          websiteId: websiteId === "all" ? 0 : Number(websiteId),
          resellerId: session?.role === "master" && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        listAllWebsites(),
        session?.role === "master"
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return { violations, websites, resellers: resellers.items }
    },
  )
  const rows = data?.violations.items ?? []

  return (
    <>
      <PageHeader title="Violations" description="Kept for 7 days, then removed." />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="violation-search">Search</FieldLabel>
          <Input
            id="violation-search"
            value={query}
            placeholder="Username or IP"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="violation-website">Website</FieldLabel>
          <ChoiceSelect
            id="violation-website"
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
        {session?.role === "master" ? (
          <Field className="md:w-64">
            <FieldLabel htmlFor="violation-reseller">Reseller</FieldLabel>
            <ChoiceSelect
              id="violation-reseller"
              value={resellerId}
              onValueChange={(value) => {
                setResellerId(value)
                setPage(1)
              }}
              items={[
                { label: "All resellers", value: "all" },
                ...(data?.resellers ?? []).map((reseller) => ({ label: reseller.username, value: String(reseller.id) })),
              ]}
            />
          </Field>
        ) : null}
      </div>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.violations.page ?? page,
              total: data?.violations.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${websiteId}-${resellerId}`}
            emptyTitle={query || websiteId !== "all" ? "No matches" : "No violations"}
            emptyDescription="Search by username or client IP."
          />
        </CardContent>
      </Card>
    </>
  )
}
