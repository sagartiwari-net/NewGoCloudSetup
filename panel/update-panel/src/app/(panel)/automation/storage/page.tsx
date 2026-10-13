"use client"

import { useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { SecretTextarea } from "@/components/secret-field"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { getAutomationStorage, listAutomationStorage, saveAutomationStorage } from "@/lib/api/client"
import type { AutomationStorageDetail, AutomationStorageRow } from "@/lib/api/types"
import { formatIst } from "@/lib/format"
import { runMutation } from "@/lib/mutate"
import { useListPage } from "@/lib/page-size"

const schema = z.object({
  cookie: z.string().trim().min(1, "Enter a cookie"),
})

type FormValues = z.infer<typeof schema>

export default function AutomationStoragePage() {
  const [query, setQuery] = useState("")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(`automation-storage-${page}-${pageSize}-${query}`, () =>
    listAutomationStorage({ page, pageSize, query }),
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AutomationStorageRow | null>(null)
  const rows = data?.items ?? []

  const columns: DataColumn<AutomationStorageRow>[] = [
    { id: "name", header: "Credential", cell: (row) => row.name, sortValue: (row) => row.name },
    { id: "provider", header: "Provider", cell: (row) => row.provider, sortValue: (row) => row.provider },
    {
      id: "updated",
      header: "Updated",
      cell: (row) => (row.updated_at ? formatIst(row.updated_at) : "—"),
      sortValue: (row) => row.updated_at,
    },
    { id: "jobs", header: "Jobs", cell: (row) => row.job_count, sortValue: (row) => row.job_count },
    {
      id: "actions",
      header: "Cookie",
      cell: (row) => (
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setEditing(row)
            setOpen(true)
          }}
        >
          View
        </Button>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Storage"
        description="The latest portal cookie for each credential. Jobs read this before they log in."
      />
      <Field className="md:max-w-sm">
        <FieldLabel htmlFor="storage-search">Search</FieldLabel>
        <Input
          id="storage-search"
          value={query}
          placeholder="Credential or provider"
          onChange={(event) => {
            setQuery(event.target.value)
            setPage(1)
          }}
        />
      </Field>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.credential_id}
            loading={loading}
            paging={{
              page: data?.page ?? page,
              total: data?.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={query}
            emptyTitle={query ? "No matches" : "No stored sessions"}
            emptyDescription="A row appears here for each credential. The cookie is filled by a job or by hand."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? editing.name : "Session cookie"}
        description="This replaces the current portal cookie. It is not written into the log."
      >
        {open && editing ? (
          <StorageEditor
            key={editing.credential_id}
            credentialId={editing.credential_id}
            onDone={() => {
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
    </>
  )
}

function StorageEditor({ credentialId, onDone }: { credentialId: number; onDone: () => void }) {
  const { data, loading } = useResource(`automation-storage-${credentialId}`, () => getAutomationStorage(credentialId))
  if (loading || !data) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner />
        Loading cookie…
      </div>
    )
  }
  return <StorageForm detail={data} onDone={onDone} />
}

function StorageForm({ detail, onDone }: { detail: AutomationStorageDetail; onDone: () => void }) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { cookie: detail.cookie },
  })
  const errors = form.formState.errors

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={form.handleSubmit(async (values) => {
        const ok = await runMutation(
          () => saveAutomationStorage(detail.credential_id, values.cookie.trim()),
          "Cookie saved",
        )
        if (ok) onDone()
      })}
    >
      <FieldGroup>
        <Field data-invalid={Boolean(errors.cookie) || undefined}>
          <FieldLabel htmlFor="storage-cookie">Cookie</FieldLabel>
          <Controller
            control={form.control}
            name="cookie"
            render={({ field }) => (
              <SecretTextarea
                id="storage-cookie"
                value={field.value}
                onChange={field.onChange}
                invalid={Boolean(errors.cookie)}
              />
            )}
          />
          <FieldError errors={[errors.cookie]} />
        </Field>
      </FieldGroup>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button type="button" variant="outline" onClick={onDone}>
          Cancel
        </Button>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save
        </Button>
      </div>
    </form>
  )
}
