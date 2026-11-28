"use client"

import { useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useResource } from "@/hooks/use-resource"
import {
  deleteAhrefsBarPortal,
  listAhrefsBarPortals,
  saveAhrefsBarPortal,
} from "@/lib/api/client"
import type { AhrefsBarPortal } from "@/lib/api/types"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  name: z.string().trim(),
  site_url: z.string().trim().min(1, "Enter site URL"),
  cookie_name: z.string().trim().min(1, "Cookie name"),
  check_url: z.string().trim(),
  product_ids: z.string().trim(),
  shared_secret: z.string(),
  enabled: z.enum(["yes", "no"]),
})

type FormValues = z.infer<typeof schema>

export default function AhrefsBarPortalsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}`,
    () => listAhrefsBarPortals({ page, pageSize, query }),
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AhrefsBarPortal | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AhrefsBarPortal | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.items ?? []

  const columns: DataColumn<AhrefsBarPortal>[] = [
    { id: "name", header: "Name", cell: (row) => row.name || "—", sortValue: (row) => row.name },
    { id: "site", header: "Site", cell: (row) => row.site_url, sortValue: (row) => row.site_url },
    { id: "cookie", header: "Cookie", cell: (row) => row.cookie_name, sortValue: (row) => row.cookie_name },
    { id: "products", header: "Products", cell: (row) => row.product_ids || "—", sortValue: (row) => row.product_ids },
    {
      id: "status",
      header: "Enabled",
      cell: (row) => <StatusBadge status={row.enabled ? "active" : "inactive"} />,
      sortValue: (row) => (row.enabled ? 1 : 0),
    },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setEditing(row)
              setOpen(true)
            }}
          >
            Edit
          </Button>
          <Button variant="destructive" size="sm" onClick={() => setPendingDelete(row)}>
            Delete
          </Button>
        </div>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Ahrefs Bar portals"
        description="aMember / tm-check sites bound to this reseller for toolbar login verify."
        action={
          <Button
            onClick={() => {
              setEditing(null)
              setOpen(true)
            }}
          >
            <PlusIcon data-icon="inline-start" />
            Add portal
          </Button>
        }
      />
      <Field className="md:max-w-sm">
        <FieldLabel htmlFor="ab-portal-search">Search</FieldLabel>
        <Input
          id="ab-portal-search"
          value={query}
          placeholder="Name or URL"
          onChange={(e) => {
            setQuery(e.target.value)
            setPage(1)
          }}
        />
      </Field>
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{ page: data?.page ?? page, total: data?.total ?? 0, pageSize, onPageChange: setPage }}
            resetKey={query}
            emptyTitle={query ? "No matches" : "No portals"}
            emptyDescription="Bind an aMember site URL for session verify."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit portal" : "Add portal"}
        description="Site cookie + optional remote tm-check URL and shared secret."
      >
        {open ? (
          <PortalForm
            key={editing?.id ?? "new"}
            portal={editing}
            onDone={async () => {
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(next) => {
          if (!next) setPendingDelete(null)
        }}
        title="Delete portal"
        description={pendingDelete ? `Delete ${pendingDelete.site_url}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteAhrefsBarPortal(pendingDelete.id), "Portal deleted").then((ok) => {
            setDeleting(false)
            if (ok) {
              setPendingDelete(null)
              reload()
            }
          })
        }}
      />
    </>
  )
}

function PortalForm({
  portal,
  onDone,
}: {
  portal: AhrefsBarPortal | null
  onDone: () => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: portal?.name ?? "",
      site_url: portal?.site_url ?? "",
      cookie_name: portal?.cookie_name || "PHPSESSID",
      check_url: portal?.check_url ?? "",
      product_ids: portal?.product_ids ?? "",
      shared_secret: "",
      enabled: portal?.enabled === false ? "no" : "yes",
    },
  })
  const errors = form.formState.errors

  async function onSubmit(values: FormValues) {
    const ok = await runMutation(
      () =>
        saveAhrefsBarPortal({
          id: portal?.id,
          name: values.name,
          site_url: values.site_url,
          cookie_name: values.cookie_name,
          check_url: values.check_url,
          product_ids: values.product_ids,
          shared_secret: values.shared_secret.trim() || undefined,
          enabled: values.enabled === "yes",
        }),
      portal ? "Portal updated" : "Portal saved",
    )
    if (ok) await onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="ab-p-name">Name</FieldLabel>
          <Input id="ab-p-name" {...form.register("name")} />
        </Field>
        <Field data-invalid={Boolean(errors.site_url) || undefined}>
          <FieldLabel htmlFor="ab-p-site">Site URL</FieldLabel>
          <Input id="ab-p-site" placeholder="https://members.example.com" {...form.register("site_url")} />
          <FieldError errors={[errors.site_url]} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-p-cookie">Cookie name</FieldLabel>
          <Input id="ab-p-cookie" {...form.register("cookie_name")} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-p-check">tm-check URL</FieldLabel>
          <Input id="ab-p-check" placeholder="https://…/tm-check.php (optional)" {...form.register("check_url")} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-p-products">Product IDs</FieldLabel>
          <Input id="ab-p-products" placeholder="12,34" {...form.register("product_ids")} />
          <FieldDescription>Comma-separated aMember product IDs.</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-p-secret">Shared secret</FieldLabel>
          <Input
            id="ab-p-secret"
            type="password"
            placeholder={portal?.has_secret ? "Leave blank to keep current" : "Optional"}
            {...form.register("shared_secret")}
          />
        </Field>
        <Field>
          <FieldLabel>Enabled</FieldLabel>
          <Controller
            control={form.control}
            name="enabled"
            render={({ field }) => (
              <ToggleGroup
                spacing={2}
                value={[field.value]}
                onValueChange={(values) => {
                  const next = values[values.length - 1]
                  if (next === "yes" || next === "no") field.onChange(next)
                }}
              >
                <ToggleGroupItem value="yes" data-tone="positive">Yes</ToggleGroupItem>
                <ToggleGroupItem value="no" data-tone="negative">No</ToggleGroupItem>
              </ToggleGroup>
            )}
          />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save portal
        </Button>
      </FieldGroup>
    </form>
  )
}
