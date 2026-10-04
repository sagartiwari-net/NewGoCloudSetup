"use client"

import { useMemo, useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useResource } from "@/hooks/use-resource"
import { deleteReseller, listResellers, listAllWebsites, saveReseller, type Reseller } from "@/lib/api/client"
import type { Website } from "@/lib/api/types"
import { runMutation } from "@/lib/mutate"
import { useListPage } from "@/lib/page-size"

type FormValues = {
  username: string
  password: string
  status: "active" | "suspended"
  website_ids: number[]
}

export default function ResellersPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [status, setStatus] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${status}`,
    async () => {
      const [resellers, websites] = await Promise.all([
        listResellers({ page, pageSize, query, status }),
        listAllWebsites(),
      ])
      return { resellers, websites }
    },
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<Reseller | null>(null)
  const [pendingDelete, setPendingDelete] = useState<Reseller | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.resellers.items ?? []

  const columns: DataColumn<Reseller>[] = [
    { id: "username", header: "Username", cell: (row) => row.username, sortValue: (row) => row.username },
    { id: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} />, sortValue: (row) => row.status },
    {
      id: "count",
      header: "Website count",
      cell: (row) => row.website_ids.length,
      sortValue: (row) => row.website_ids.length,
    },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-3">
          <Button variant="outline" size="sm" onClick={() => { setEditing(row); setOpen(true) }}>
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
        title="Resellers"
        description="Operators who only see the websites you check for them."
        action={
          <Button onClick={() => { setEditing(null); setOpen(true) }}>
            <PlusIcon data-icon="inline-start" />
            New reseller
          </Button>
        }
      />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="reseller-search">Search</FieldLabel>
          <Input
            id="reseller-search"
            value={query}
            placeholder="Username"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="reseller-status">Status</FieldLabel>
          <ChoiceSelect
            id="reseller-status"
            value={status}
            onValueChange={(value) => {
              setStatus(value)
              setPage(1)
            }}
            items={[
              { label: "All", value: "all" },
              { label: "active", value: "active" },
              { label: "suspended", value: "suspended" },
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
              page: data?.resellers.page ?? page,
              total: data?.resellers.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${status}`}
            emptyTitle={query ? "No matches" : "No resellers"}
            emptyDescription="Create a reseller and choose which websites they can manage."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit reseller" : "New reseller"}
        description="Password is required when creating a reseller, and only when you choose to change it later. It is not stored in this mock."
      >
        {open ? (
          <ResellerForm
            key={editing?.id ?? "new"}
            reseller={editing}
            websites={data?.websites ?? []}
            onDone={async () => {
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(next) => { if (!next) setPendingDelete(null) }}
        title="Delete reseller"
        description={pendingDelete ? `Delete ${pendingDelete.username}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteReseller(pendingDelete.id), "Reseller deleted").then((ok) => {
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

function ResellerForm({
  reseller,
  websites,
  onDone,
}: {
  reseller: Reseller | null
  websites: Website[]
  onDone: () => Promise<void>
}) {
  const schema = useMemo(
    () =>
      z.object({
        username: z.string().trim().min(1, "Enter a username"),
        password: z.string(),
        status: z.enum(["active", "suspended"]),
        website_ids: z.array(z.number()).min(1, "Choose at least one website"),
      }).superRefine((value, ctx) => {
        const changing = value.password.length > 0
        if ((!reseller || changing) && value.password.length < 8) {
          ctx.addIssue({
            code: "custom",
            path: ["password"],
            message: "Use at least 8 characters",
          })
        }
      }),
    [reseller]
  )

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      username: reseller?.username ?? "",
      password: "",
      status: reseller?.status ?? "active",
      website_ids: reseller?.website_ids ?? [],
    },
  })
  const errors = form.formState.errors
  const [selected, setSelected] = useState(reseller?.website_ids ?? [])

  async function onSubmit(values: FormValues) {
    const ok = await runMutation(
      () =>
        saveReseller({
          id: reseller?.id,
          username: values.username,
          status: values.status,
          website_ids: values.website_ids,
        }),
      reseller ? "Reseller updated" : "Reseller saved"
    )
    if (ok) await onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.username) || undefined}>
          <FieldLabel htmlFor="username">Username</FieldLabel>
          <Input id="username" aria-invalid={Boolean(errors.username) || undefined} {...form.register("username")} />
          <FieldError errors={[errors.username]} />
        </Field>
        <Field data-invalid={Boolean(errors.password) || undefined}>
          <FieldLabel htmlFor="password">{reseller ? "New password" : "Password"}</FieldLabel>
          <Input id="password" type="password" autoComplete="new-password" aria-invalid={Boolean(errors.password) || undefined} {...form.register("password")} />
          <FieldDescription>
            {reseller ? "Leave blank to keep the current password." : "At least 8 characters."}
          </FieldDescription>
          <FieldError errors={[errors.password]} />
        </Field>
        <Field>
          <FieldLabel>Status</FieldLabel>
          <Controller
            control={form.control}
            name="status"
            render={({ field }) => (
              <ToggleGroup
                spacing={2}
                value={[field.value]}
                onValueChange={(values) => {
                  const next = values[values.length - 1]
                  if (next === "active" || next === "suspended") field.onChange(next)
                }}
              >
                <ToggleGroupItem value="active" data-tone="positive">Active</ToggleGroupItem>
                <ToggleGroupItem value="suspended" data-tone="negative">Suspended</ToggleGroupItem>
              </ToggleGroup>
            )}
          />
        </Field>
        <FieldSet>
          <FieldLegend variant="label">Websites</FieldLegend>
          <FieldDescription>This reseller can manage only the websites you check.</FieldDescription>
          <FieldGroup>
            {websites.map((website) => {
              const checked = selected.includes(website.id)
              return (
                <Field key={website.id} orientation="horizontal">
                  <Checkbox
                    id={`website-${website.id}`}
                    checked={checked}
                    onCheckedChange={(next) => {
                      const current = form.getValues("website_ids")
                      const websiteIds = next
                        ? [...current, website.id]
                        : current.filter((id) => id !== website.id)
                      setSelected(websiteIds)
                      form.setValue("website_ids", websiteIds, { shouldValidate: true })
                    }}
                  />
                  <FieldLabel htmlFor={`website-${website.id}`}>{website.name}</FieldLabel>
                </Field>
              )
            })}
          </FieldGroup>
          <FieldError errors={[errors.website_ids]} />
        </FieldSet>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save reseller
        </Button>
      </FieldGroup>
    </form>
  )
}
