"use client"

import { useState } from "react"
import { toast } from "sonner"

import { AccessToolPicker } from "@/components/access-tool-picker"
import { useAuth } from "@/components/auth-provider"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { clearAccessRejects, listAccessRejects, listWebsiteChoices, saveAccessSite } from "@/lib/api/client"
import type { AccessReject, Website } from "@/lib/api/types"
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
  const [allowing, setAllowing] = useState<AccessReject | null>(null)
  const [issuedKey, setIssuedKey] = useState("")
  const { data: websites } = useResource(`${session?.role ?? "none"}-websites`, listWebsiteChoices)
  const rows = data?.items ?? []
  const master = session?.role === "master"
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
    {
      id: "actions",
      header: "",
      cell: (row) =>
        master && row.reason === "not_whitelisted" && row.site_domain ? (
          <Button variant="outline" size="sm" onClick={() => setAllowing(row)}>
            Allow
          </Button>
        ) : null,
    },
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
      {issuedKey ? (
        <Alert>
          <AlertTitle>Copy this key now</AlertTitle>
          <AlertDescription>
            Put it in gt4acc.php. It is not shown again.
            <span className="mt-2 block font-mono break-all">{issuedKey}</span>
          </AlertDescription>
        </Alert>
      ) : null}
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
      <FormOverlay
        open={allowing != null}
        onOpenChange={(next) => { if (!next) setAllowing(null) }}
        title="Allow this website"
        description="Choose which tools it can open. The key is shown once."
      >
        {allowing ? (
          <AllowSiteForm
            key={allowing.site_domain}
            domain={allowing.site_domain}
            toolSlug={allowing.tool_slug}
            websites={websites ?? []}
            onDone={async (key) => {
              if (key) setIssuedKey(key)
              setAllowing(null)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
    </>
  )
}

function AllowSiteForm({
  domain,
  toolSlug,
  websites,
  onDone,
}: {
  domain: string
  toolSlug: string
  websites: Website[]
  onDone: (key?: string) => Promise<void>
}) {
  const preset = websites.filter((website) => website.access_slug && website.access_slug === toolSlug).map((website) => website.id)
  const [selected, setSelected] = useState(preset)
  const [saving, setSaving] = useState(false)

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        setSaving(true)
        void saveAccessSite({ domain, website_ids: selected, enabled: true })
          .then((saved) => onDone(saved.key))
          .catch((error: unknown) => {
            toast.error(error instanceof Error ? error.message : "Could not allow this website")
          })
          .finally(() => setSaving(false))
      }}
    >
      <FieldGroup>
        <Field>
          <FieldLabel>Website</FieldLabel>
          <Input value={domain} readOnly />
        </Field>
        <AccessToolPicker websites={websites} selected={selected} onChange={setSelected} />
        <Button type="submit" disabled={saving}>
          {saving ? <Spinner data-icon="inline-start" /> : null}
          Allow and create key
        </Button>
      </FieldGroup>
    </form>
  )
}
