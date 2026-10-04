"use client"

import { useState } from "react"
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
import { deleteProxy, listProxies, saveProxy } from "@/lib/api/client"
import type { ProxyEndpoint } from "@/lib/api/types"
import { maskEndpoint } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const proxyTypes = ["SOCKS5", "HTTP", "HTTPS"] as const

function buildProxyEndpoint(proxyType: (typeof proxyTypes)[number], raw: string) {
  const scheme = proxyType === "SOCKS5" ? "socks5" : proxyType.toLowerCase()
  const value = raw.trim()
  if (!value) return ""
  if (value.includes("://")) {
    let parsed: URL
    try {
      parsed = new URL(value)
    } catch {
      return ""
    }
    const host = parsed.hostname
    const port = parsed.port
    if (!host || !/^\d+$/.test(port)) return ""
    const user = decodeURIComponent(parsed.username)
    const pass = decodeURIComponent(parsed.password)
    if (user || pass) {
      return `${scheme}://${encodeURIComponent(user)}:${encodeURIComponent(pass)}@${host}:${port}`
    }
    return `${scheme}://${host}:${port}`
  }
  const parts = value.split(":")
  const host = parts[0]?.trim() ?? ""
  const port = parts[1]?.trim() ?? ""
  if (!host || !/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535) return ""
  if (parts.length === 2) return `${scheme}://${host}:${port}`
  if (parts.length < 4) return ""
  const user = parts[2]
  const pass = parts.slice(3).join(":")
  if (!user || !pass) return ""
  return `${scheme}://${encodeURIComponent(user)}:${encodeURIComponent(pass)}@${host}:${port}`
}

const schema = z.object({
  name: z.string().trim().min(1, "Enter a name"),
  proxy_type: z.enum(proxyTypes),
  endpoint: z.string().trim().min(1, "Enter an endpoint"),
  status: z.enum(["active", "inactive"]),
}).superRefine((value, ctx) => {
  if (!buildProxyEndpoint(value.proxy_type, value.endpoint)) {
    ctx.addIssue({
      code: "custom",
      path: ["endpoint"],
      message: "Use host:port:user:pass, or host:port",
    })
  }
})

type FormValues = z.infer<typeof schema>

export default function ProxiesPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [status, setStatus] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${status}`,
    () => listProxies({ page, pageSize, query, status }),
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<ProxyEndpoint | null>(null)
  const [pendingDelete, setPendingDelete] = useState<ProxyEndpoint | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.items ?? []

  const columns: DataColumn<ProxyEndpoint>[] = [
    { id: "name", header: "Name", cell: (row) => row.name, sortValue: (row) => row.name },
    { id: "type", header: "Type", cell: (row) => row.proxy_type, sortValue: (row) => row.proxy_type },
    { id: "endpoint", header: "Endpoint", cell: (row) => maskEndpoint(row.endpoint), sortValue: (row) => row.name },
    { id: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} />, sortValue: (row) => row.status },
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
        title="Proxy Manager"
        description="Shared endpoints that mapped accounts can use."
        action={
          <Button onClick={() => { setEditing(null); setOpen(true) }}>
            <PlusIcon data-icon="inline-start" />
            New proxy
          </Button>
        }
      />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="proxy-search">Search</FieldLabel>
          <Input
            id="proxy-search"
            value={query}
            placeholder="Name or endpoint"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="proxy-status">Status</FieldLabel>
          <ChoiceSelect
            id="proxy-status"
            value={status}
            onValueChange={(value) => {
              setStatus(value)
              setPage(1)
            }}
            items={[
              { label: "All", value: "all" },
              { label: "active", value: "active" },
              { label: "inactive", value: "inactive" },
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
            paging={{ page: data?.page ?? page, total: data?.total ?? 0, pageSize, onPageChange: setPage }}
            resetKey={`${query}-${status}`}
            emptyTitle={query ? "No matches" : "No proxies"}
            emptyDescription="Add a SOCKS5, HTTP, or HTTPS endpoint for the account forms."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit proxy" : "New proxy"}
        description="Passwords inside the endpoint stay available here so you can edit them. The table shows a mask."
      >
        {open ? (
          <ProxyForm
            key={editing?.id ?? "new"}
            proxy={editing}
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
        title="Delete proxy"
        description={pendingDelete ? `Delete ${pendingDelete.name}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteProxy(pendingDelete.id), "Proxy deleted").then((ok) => {
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

function ProxyForm({
  proxy,
  onDone,
}: {
  proxy: ProxyEndpoint | null
  onDone: () => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: proxy?.name ?? "",
      proxy_type: proxy?.proxy_type ?? "SOCKS5",
      endpoint: proxy?.endpoint ?? "",
      status: proxy?.status ?? "active",
    },
  })
  const errors = form.formState.errors

  async function onSubmit(values: FormValues) {
    const endpoint = buildProxyEndpoint(values.proxy_type, values.endpoint)
    if (!endpoint) return
    const ok = await runMutation(
      () => saveProxy({ id: proxy?.id, ...values, endpoint }),
      proxy ? "Proxy updated" : "Proxy saved"
    )
    if (ok) await onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="name">Name</FieldLabel>
          <Input id="name" aria-invalid={Boolean(errors.name) || undefined} {...form.register("name")} />
          <FieldError errors={[errors.name]} />
        </Field>
        <Field>
          <FieldLabel>Type</FieldLabel>
          <Controller
            control={form.control}
            name="proxy_type"
            render={({ field }) => (
              <ToggleGroup
                spacing={2}
                value={[field.value]}
                onValueChange={(values) => {
                  const next = values[values.length - 1]
                  if (next === "SOCKS5" || next === "HTTP" || next === "HTTPS") field.onChange(next)
                }}
              >
                <ToggleGroupItem value="SOCKS5">SOCKS5</ToggleGroupItem>
                <ToggleGroupItem value="HTTP">HTTP</ToggleGroupItem>
                <ToggleGroupItem value="HTTPS">HTTPS</ToggleGroupItem>
              </ToggleGroup>
            )}
          />
        </Field>
        <Field data-invalid={Boolean(errors.endpoint) || undefined}>
          <FieldLabel htmlFor="endpoint">Endpoint</FieldLabel>
          <Input id="endpoint" className="font-mono" placeholder="216.224.127.122:12323:user:pass" aria-invalid={Boolean(errors.endpoint) || undefined} {...form.register("endpoint")} />
          <FieldDescription>Paste host:port:user:pass. The type above turns it into SOCKS5, HTTP, or HTTPS.</FieldDescription>
          <FieldError errors={[errors.endpoint]} />
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
                  if (next === "active" || next === "inactive") field.onChange(next)
                }}
              >
                <ToggleGroupItem value="active" data-tone="positive">Active</ToggleGroupItem>
                <ToggleGroupItem value="inactive" data-tone="negative">Inactive</ToggleGroupItem>
              </ToggleGroup>
            )}
          />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save proxy
        </Button>
      </FieldGroup>
    </form>
  )
}
