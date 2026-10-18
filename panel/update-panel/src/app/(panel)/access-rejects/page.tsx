"use client"

import { useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import { clearAccessRejects, listAccessRejects } from "@/lib/api/client"
import type { AccessReject } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { useListPage } from "@/lib/page-size"

export default function AccessRejectsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}`,
    () => listAccessRejects({ page, pageSize, query }),
  )
  const [confirmClear, setConfirmClear] = useState(false)
  const [clearing, setClearing] = useState(false)
  const rows = data?.items ?? []
  const columns: DataColumn<AccessReject>[] = [
    { id: "time", header: "Time", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
    {
      id: "website",
      header: "Website",
      cell: (row) => row.website_name || row.site_domain || "—",
      sortValue: (row) => row.website_name || row.site_domain,
    },
    { id: "user", header: "Username", cell: (row) => row.username || "—", sortValue: (row) => row.username },
    { id: "ip", header: "IP", cell: (row) => row.client_ip || "—", sortValue: (row) => row.client_ip },
    { id: "message", header: "Reason", cell: (row) => row.message, sortValue: (row) => row.message },
  ]

  return (
    <>
      <PageHeader
        title="Access rejects"
        description="Why a button was refused. Rows stay for 7 days. A reseller only sees their own websites."
        action={
          <Button variant="outline" onClick={() => setConfirmClear(true)} disabled={rows.length === 0 && (data?.total ?? 0) === 0}>
            Clear
          </Button>
        }
      />
      <Card>
        <CardContent className="flex flex-col gap-4">
          <Field className="max-w-sm">
            <FieldLabel htmlFor="reject-search">Search</FieldLabel>
            <Input
              id="reject-search"
              value={query}
              placeholder="Username, website, or reason"
              onChange={(event) => {
                setQuery(event.target.value)
                setPage(1)
              }}
            />
          </Field>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            resetKey={`${query}-${page}`}
            paging={{
              page: data?.page ?? page,
              total: data?.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            emptyTitle="No rejects"
            emptyDescription="Refused button clicks show up here."
          />
        </CardContent>
      </Card>
      <ConfirmDialog
        open={confirmClear}
        onOpenChange={setConfirmClear}
        title="Clear these rejects?"
        description={session?.role === "master" ? "This removes every reject, including other websites." : "This removes rejects for your websites only."}
        confirmLabel="Clear"
        pending={clearing}
        onConfirm={() => {
          setClearing(true)
          void clearAccessRejects().finally(() => {
            setClearing(false)
            setConfirmClear(false)
            reload()
          })
        }}
      />
    </>
  )
}
