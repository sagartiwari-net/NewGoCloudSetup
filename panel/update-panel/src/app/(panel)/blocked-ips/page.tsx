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
import { IpLink } from "@/components/ip-link"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { useResource } from "@/hooks/use-resource"
import { blockIP, listBlockedIPs, listAllWebsites, unblockIP } from "@/lib/api/client"
import type { BlockedIP, Website } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  client_ip: z.string().trim().refine((value) => {
    return z.ipv4().safeParse(value).success || z.ipv6().safeParse(value).success
  }, "Enter a valid IPv4 or IPv6 address"),
  website_id: z.string().min(1, "Choose a scope"),
  reason: z.string().trim().min(1, "Enter a reason"),
})

type FormValues = z.infer<typeof schema>

const columnsBase: DataColumn<BlockedIP>[] = [
  { id: "ip", header: "IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
  { id: "scope", header: "Scope", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
  { id: "reason", header: "Reason", cell: (row) => row.reason, sortValue: (row) => row.reason },
  { id: "created", header: "Created", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
]

export default function BlockedIpsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [websiteId, setWebsiteId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${websiteId}`,
    async () => {
      const [blocked, websites] = await Promise.all([
        listBlockedIPs({
          page,
          pageSize,
          query,
          websiteId: websiteId === "all" ? 0 : Number(websiteId),
        }),
        listAllWebsites(),
      ])
      return { blocked, websites }
    },
  )
  const [open, setOpen] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<BlockedIP | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.blocked.items ?? []

  const columns: DataColumn<BlockedIP>[] = [
    ...columnsBase,
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <Button variant="destructive" size="sm" onClick={() => setPendingDelete(row)}>
          Unblock
        </Button>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Blocked IPs"
        description="Stop a client IP on one website or, for master, on every website."
        action={
          <Button onClick={() => setOpen(true)}>
            <PlusIcon data-icon="inline-start" />
            Block IP
          </Button>
        }
      />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="ip-search">Search</FieldLabel>
          <Input
            id="ip-search"
            value={query}
            placeholder="IP or reason"
            onChange={(event) => {
              setQuery(event.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="ip-website">Website</FieldLabel>
          <ChoiceSelect
            id="ip-website"
            value={websiteId}
            onValueChange={(value) => {
              setWebsiteId(value)
              setPage(1)
            }}
            items={[
              { label: "All websites", value: "all" },
              ...(data?.websites ?? []).map((website) => ({ label: website.name, value: String(website.id) })),
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
              page: data?.blocked.page ?? page,
              total: data?.blocked.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${websiteId}`}
            emptyTitle={query ? "No matches" : "No blocked IPs"}
            emptyDescription="Block an address that should not reach a tool."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title="Block IP"
        description="IPv4 and IPv6 are both accepted."
      >
        {open ? (
          <BlockForm
            websites={data?.websites ?? []}
            allowAll={session?.role === "master"}
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
        title="Remove block"
        description={pendingDelete ? `Unblock ${pendingDelete.client_ip}?` : ""}
        confirmLabel="Unblock"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => unblockIP(pendingDelete.id), "IP unblocked").then((ok) => {
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

function BlockForm({
  websites,
  allowAll,
  onDone,
}: {
  websites: Website[]
  allowAll: boolean
  onDone: () => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      client_ip: "",
      website_id: websites[0] ? String(websites[0].id) : allowAll ? "0" : "",
      reason: "",
    },
  })

  async function onSubmit(values: FormValues) {
    const ok = await runMutation(
      () =>
        blockIP({
          website_id: Number(values.website_id),
          client_ip: values.client_ip,
          reason: values.reason,
        }),
      "IP blocked"
    )
    if (ok) await onDone()
  }

  const errors = form.formState.errors
  const items = [
    ...(allowAll ? [{ label: "All websites", value: "0" }] : []),
    ...websites.map((website) => ({ label: website.name, value: String(website.id) })),
  ]

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.client_ip) || undefined}>
          <FieldLabel htmlFor="client_ip">IP</FieldLabel>
          <Input id="client_ip" className="font-mono" aria-invalid={Boolean(errors.client_ip) || undefined} {...form.register("client_ip")} />
          <FieldError errors={[errors.client_ip]} />
        </Field>
        <Field data-invalid={Boolean(errors.website_id) || undefined}>
          <FieldLabel htmlFor="website_id">Website scope</FieldLabel>
          <Controller
            control={form.control}
            name="website_id"
            render={({ field }) => (
              <ChoiceSelect id="website_id" value={field.value} onValueChange={field.onChange} items={items} />
            )}
          />
          <FieldError errors={[errors.website_id]} />
        </Field>
        <Field data-invalid={Boolean(errors.reason) || undefined}>
          <FieldLabel htmlFor="reason">Reason</FieldLabel>
          <Textarea id="reason" aria-invalid={Boolean(errors.reason) || undefined} {...form.register("reason")} />
          <FieldError errors={[errors.reason]} />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Block IP
        </Button>
      </FieldGroup>
    </form>
  )
}
