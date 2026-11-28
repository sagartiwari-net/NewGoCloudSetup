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
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useResource } from "@/hooks/use-resource"
import {
  currentOperator,
  deleteAhrefsBarPortal,
  listAhrefsBarPortals,
  listResellerChoices,
  saveAhrefsBarPortal,
} from "@/lib/api/client"
import type { AhrefsBarPortal } from "@/lib/api/types"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

export default function AhrefsBarPortalsPage() {
  const { session } = useAuth()
  const isMaster = session?.role === "master"
  const [query, setQuery] = useState("")
  const [filterResellerId, setFilterResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${filterResellerId}`,
    () =>
      listAhrefsBarPortals({
        page,
        pageSize,
        query,
        resellerId: isMaster && filterResellerId !== "all" ? Number(filterResellerId) : undefined,
      }),
  )
  const { data: filterChoices } = useResource(isMaster ? "ab-portal-filter-resellers" : "skip", async () => {
    if (!isMaster) return { items: [] as Array<{ id: number; username: string }> }
    const page = await listResellerChoices()
    return { items: page.items.filter((r) => r.ahrefs_bar_enabled).map((r) => ({ id: r.id, username: r.username })) }
  })
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AhrefsBarPortal | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AhrefsBarPortal | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.items ?? []

  const filterItems = useMemo(
    () => [
      { label: "All resellers", value: "all" },
      ...(filterChoices?.items ?? []).map((r) => ({ label: r.username, value: String(r.id) })),
    ],
    [filterChoices],
  )

  const columns: DataColumn<AhrefsBarPortal>[] = [
    { id: "name", header: "Name", cell: (row) => row.name || "—", sortValue: (row) => row.name },
    ...(isMaster
      ? [
          {
            id: "reseller",
            header: "Reseller",
            cell: (row: AhrefsBarPortal) => row.reseller_username || `id ${row.reseller_id}`,
            sortValue: (row: AhrefsBarPortal) => row.reseller_username || String(row.reseller_id),
          } satisfies DataColumn<AhrefsBarPortal>,
        ]
      : []),
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
        description="aMember / tm-check sites bound to one reseller — that reseller's stamped extension can only use these hosts."
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
      <div className="flex flex-wrap gap-4">
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
        {isMaster ? (
          <Field className="md:max-w-xs">
            <FieldLabel htmlFor="ab-portal-reseller-filter">Reseller</FieldLabel>
            <ChoiceSelect
              id="ab-portal-reseller-filter"
              value={filterResellerId}
              onValueChange={(v) => {
                setFilterResellerId(v)
                setPage(1)
              }}
              items={filterItems}
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
            paging={{ page: data?.page ?? page, total: data?.total ?? 0, pageSize, onPageChange: setPage }}
            resetKey={`${query}-${filterResellerId}`}
            emptyTitle={query || filterResellerId !== "all" ? "No matches" : "No portals"}
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
            isMaster={isMaster}
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
  isMaster,
  onDone,
}: {
  portal: AhrefsBarPortal | null
  isMaster: boolean
  onDone: () => Promise<void>
}) {
  const schema = useMemo(
    () =>
      z
        .object({
          reseller_id: z.string(),
          name: z.string().trim(),
          site_url: z.string().trim().min(1, "Enter site URL"),
          cookie_name: z.string().trim().min(1, "Cookie name"),
          check_url: z.string().trim(),
          product_ids: z.string().trim(),
          shared_secret: z.string(),
          enabled: z.enum(["yes", "no"]),
        })
        .superRefine((value, ctx) => {
          if (isMaster && (!value.reseller_id || value.reseller_id === "0")) {
            ctx.addIssue({ code: "custom", path: ["reseller_id"], message: "Choose a reseller" })
          }
        }),
    [isMaster],
  )
  type FormValues = z.infer<typeof schema>

  const { data: resellerChoices } = useResource(isMaster ? "ab-portal-reseller-choices" : "skip", async () => {
    if (!isMaster) {
      return { me: null as null | { id: number; username: string }, items: [] as Array<{ id: number; username: string }> }
    }
    const [me, page] = await Promise.all([currentOperator(), listResellerChoices()])
    return {
      me: { id: me.id, username: me.username },
      items: page.items.filter((r) => r.ahrefs_bar_enabled).map((r) => ({ id: r.id, username: r.username })),
    }
  })

  const resellerItems = useMemo(() => {
    const items: Array<{ label: string; value: string }> = []
    if (resellerChoices?.me) {
      items.push({ label: `Master (${resellerChoices.me.username})`, value: String(resellerChoices.me.id) })
    }
    for (const r of resellerChoices?.items ?? []) {
      items.push({ label: r.username, value: String(r.id) })
    }
    return items
  }, [resellerChoices])

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      reseller_id: portal?.reseller_id ? String(portal.reseller_id) : "",
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
          reseller_id: isMaster ? Number(values.reseller_id) : undefined,
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
        {isMaster ? (
          <Field data-invalid={Boolean(errors.reseller_id) || undefined}>
            <FieldLabel htmlFor="ab-p-reseller">Reseller</FieldLabel>
            <Controller
              control={form.control}
              name="reseller_id"
              render={({ field }) => (
                <ChoiceSelect
                  id="ab-p-reseller"
                  value={field.value}
                  onValueChange={field.onChange}
                  placeholder="Choose reseller"
                  items={resellerItems}
                />
              )}
            />
            <FieldDescription>
              Required. Only that reseller&apos;s stamped extension can login via this aMember site.
            </FieldDescription>
            <FieldError errors={[errors.reseller_id]} />
          </Field>
        ) : null}
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
          <FieldDescription>
            aMember Pro is often <span className="font-mono">amember_nr</span>. Wrong name → bar stays unauthorized while the site looks logged in.
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-p-check">tm-check URL</FieldLabel>
          <Input id="ab-p-check" placeholder="https://site.example/tm-check.php" {...form.register("check_url")} />
          <FieldDescription>
            Full URL to tm-check.php on that site. File must sit beside <span className="font-mono">library/Am/Lite.php</span> (path differs per reseller).
          </FieldDescription>
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
