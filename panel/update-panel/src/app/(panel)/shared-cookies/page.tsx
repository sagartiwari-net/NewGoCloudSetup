"use client"

import { useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { useResource } from "@/hooks/use-resource"
import { deleteCookieLink, listCookieLinks } from "@/lib/api/client"
import type { CookieLink } from "@/lib/api/types"
import { formatIst } from "@/lib/format"
import { runMutation } from "@/lib/mutate"

export default function SharedCookiesPage() {
  const { session } = useAuth()
  const { data, loading, reload } = useResource(session?.role ?? "none", () =>
    session?.role === "master" ? listCookieLinks() : Promise.resolve({ items: [] }),
  )
  const [removing, setRemoving] = useState<CookieLink | null>(null)
  const [pending, setPending] = useState(false)
  const rows = data?.items ?? []

  const columns: DataColumn<CookieLink>[] = [
    {
      id: "follower",
      header: "Account",
      cell: (row) => row.follower_label,
      sortValue: (row) => row.follower_label,
    },
    {
      id: "source",
      header: "Uses",
      cell: (row) => row.source_label,
      sortValue: (row) => row.source_label,
    },
    {
      id: "since",
      header: "Since",
      cell: (row) => (row.created_at ? formatIst(row.created_at) : "—"),
      sortValue: (row) => row.created_at,
    },
    {
      id: "stop",
      header: "Link",
      cell: (row) => (
        <Button variant="outline" size="sm" onClick={() => setRemoving(row)}>
          Stop using
        </Button>
      ),
    },
  ]

  if (session?.role !== "master") return null

  return (
    <>
      <PageHeader
        title="Shared cookies"
        description="Which account uses another account's cookie. Stopping a link leaves the last cookie in place."
      />
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.follower_account_id}
            loading={loading}
            emptyTitle="No shared cookies"
            emptyDescription="Open an account and choose Use from account to start a link."
          />
        </CardContent>
      </Card>
      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => {
          if (!open) setRemoving(null)
        }}
        title="Stop using this cookie?"
        description={
          removing
            ? `${removing.follower_label} keeps the cookie it has now. Later updates of ${removing.source_label} will not copy over.`
            : ""
        }
        confirmLabel="Stop using"
        pending={pending}
        onConfirm={() => {
          if (!removing) return
          setPending(true)
          void runMutation(
            () => deleteCookieLink(removing.follower_account_id),
            "Stopped. That account keeps the cookie it has now.",
          ).then((ok) => {
            setPending(false)
            if (!ok) return
            setRemoving(null)
            reload()
          })
        }}
      />
    </>
  )
}
