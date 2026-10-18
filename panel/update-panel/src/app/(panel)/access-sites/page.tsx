"use client"

import { useState } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { deleteAccessSite, listAccessSites, saveAccessSite } from "@/lib/api/client"
import type { AccessSite } from "@/lib/api/types"
import { formatTime } from "@/lib/format"

const schema = z.object({
  domain: z.string().trim().min(1, "Enter the aMember domain"),
  note: z.string().trim().max(200, "Use 200 characters or fewer"),
})

type FormValues = z.infer<typeof schema>

export default function AccessSitesPage() {
  const { session } = useAuth()
  const { data, loading, reload } = useResource(session?.role ?? "none", listAccessSites)
  const [open, setOpen] = useState(false)
  const [issuedKey, setIssuedKey] = useState("")
  const [pendingDelete, setPendingDelete] = useState<AccessSite | null>(null)
  const [deleting, setDeleting] = useState(false)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { domain: "", note: "" },
  })

  const rows = data ?? []
  const columns: DataColumn<AccessSite>[] = [
    { id: "domain", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
    { id: "note", header: "Note", cell: (row) => row.note || "—", sortValue: (row) => row.note },
    { id: "enabled", header: "Status", cell: (row) => (row.enabled ? "Allowed" : "Off"), sortValue: (row) => (row.enabled ? "1" : "0") },
    { id: "created", header: "Added", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void saveAccessSite({ id: row.id, enabled: !row.enabled, note: row.note }).then(() => reload())
            }}
          >
            {row.enabled ? "Turn off" : "Turn on"}
          </Button>
          <Button variant="destructive" size="sm" onClick={() => setPendingDelete(row)}>
            Remove
          </Button>
        </div>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Allowed sites"
        description="Only these aMember websites can ask the panel to open a tool."
        action={
          <Button onClick={() => { form.reset(); setOpen(true) }}>
            <PlusIcon data-icon="inline-start" />
            Add site
          </Button>
        }
      />
      {issuedKey ? (
        <Alert>
          <AlertTitle>Copy this key now</AlertTitle>
          <AlertDescription>
            Put it in the aMember file. It is not shown again.
            <span className="mt-2 block font-mono break-all">{issuedKey}</span>
          </AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            emptyTitle="No allowed sites"
            emptyDescription="Add an aMember domain before its buttons can open a tool."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title="Add site"
        description="The key is created here and shown once."
      >
        <form
          onSubmit={form.handleSubmit(async (values) => {
            const saved = await saveAccessSite({ domain: values.domain, note: values.note })
            if (saved.key) setIssuedKey(saved.key)
            setOpen(false)
            reload()
          })}
        >
          <FieldGroup>
            <Field data-invalid={Boolean(form.formState.errors.domain) || undefined}>
              <FieldLabel htmlFor="domain">aMember domain</FieldLabel>
              <Input id="domain" placeholder="toolsmandi.com" {...form.register("domain")} />
              <FieldError errors={[form.formState.errors.domain]} />
            </Field>
            <Field data-invalid={Boolean(form.formState.errors.note) || undefined}>
              <FieldLabel htmlFor="note">Note</FieldLabel>
              <Input id="note" {...form.register("note")} />
              <FieldError errors={[form.formState.errors.note]} />
            </Field>
            <Button type="submit" disabled={form.formState.isSubmitting}>
              {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
              Create key
            </Button>
          </FieldGroup>
        </form>
      </FormOverlay>
      <ConfirmDialog
        open={pendingDelete != null}
        onOpenChange={(next) => { if (!next) setPendingDelete(null) }}
        title="Remove this site?"
        description="Its buttons stop working until you add it again. The old key will not work."
        confirmLabel="Remove"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void deleteAccessSite(pendingDelete.id).finally(() => {
            setDeleting(false)
            setPendingDelete(null)
            reload()
          })
        }}
      />
    </>
  )
}
