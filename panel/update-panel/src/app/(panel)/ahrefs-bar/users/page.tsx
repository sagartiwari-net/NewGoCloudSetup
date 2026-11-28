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
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import {
  deleteAhrefsBarUserLimit,
  listAhrefsBarUserLimits,
  saveAhrefsBarUserLimit,
} from "@/lib/api/client"
import type { AhrefsBarUserLimit } from "@/lib/api/types"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  amember_login: z.string().trim().min(1, "Enter login"),
  hourly_limit: z.number().int().min(0),
  daily_limit: z.number().int().min(0),
})

type FormValues = z.infer<typeof schema>

export default function AhrefsBarUsersPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}`,
    () => listAhrefsBarUserLimits({ page, pageSize, query }),
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AhrefsBarUserLimit | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AhrefsBarUserLimit | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.items ?? []

  const columns: DataColumn<AhrefsBarUserLimit>[] = [
    { id: "login", header: "aMember login", cell: (row) => row.amember_login, sortValue: (row) => row.amember_login },
    { id: "hourly", header: "Hourly", cell: (row) => row.hourly_limit, sortValue: (row) => row.hourly_limit },
    { id: "daily", header: "Daily", cell: (row) => row.daily_limit, sortValue: (row) => row.daily_limit },
    { id: "updated", header: "Updated", cell: (row) => row.updated_at || "—", sortValue: (row) => row.updated_at },
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
        title="Ahrefs Bar user limits"
        description="Custom hourly/daily overrides per aMember login. Others use reseller defaults."
        action={
          <Button
            onClick={() => {
              setEditing(null)
              setOpen(true)
            }}
          >
            <PlusIcon data-icon="inline-start" />
            Add override
          </Button>
        }
      />
      <Field className="md:max-w-sm">
        <FieldLabel htmlFor="ab-ul-search">Search</FieldLabel>
        <Input
          id="ab-ul-search"
          value={query}
          placeholder="Login"
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
            emptyTitle={query ? "No matches" : "No custom limits"}
            emptyDescription="Add a per-user override when defaults are not enough."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit override" : "Add override"}
        description="Sets both hourly and daily caps for this login."
      >
        {open ? (
          <UserLimitForm
            key={editing?.id ?? "new"}
            row={editing}
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
        title="Delete override"
        description={pendingDelete ? `Remove custom limits for ${pendingDelete.amember_login}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteAhrefsBarUserLimit(pendingDelete.id), "Override deleted").then((ok) => {
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

function UserLimitForm({
  row,
  onDone,
}: {
  row: AhrefsBarUserLimit | null
  onDone: () => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      amember_login: row?.amember_login ?? "",
      hourly_limit: row?.hourly_limit ?? 50,
      daily_limit: row?.daily_limit ?? 150,
    },
  })
  const errors = form.formState.errors

  async function onSubmit(values: FormValues) {
    const ok = await runMutation(
      () =>
        saveAhrefsBarUserLimit({
          id: row?.id,
          amember_login: values.amember_login,
          hourly_limit: values.hourly_limit,
          daily_limit: values.daily_limit,
        }),
      row ? "Override updated" : "Override saved",
    )
    if (ok) await onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.amember_login) || undefined}>
          <FieldLabel htmlFor="ab-ul-login">aMember login</FieldLabel>
          <Input id="ab-ul-login" {...form.register("amember_login")} />
          <FieldError errors={[errors.amember_login]} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-ul-hourly">Hourly limit</FieldLabel>
          <Input id="ab-ul-hourly" type="number" {...form.register("hourly_limit", { valueAsNumber: true })} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-ul-daily">Daily limit</FieldLabel>
          <Input id="ab-ul-daily" type="number" {...form.register("daily_limit", { valueAsNumber: true })} />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save
        </Button>
      </FieldGroup>
    </form>
  )
}
