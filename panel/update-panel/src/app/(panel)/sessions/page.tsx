"use client"

import { useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { ListFilters } from "@/components/list-filters"
import { PageHeader } from "@/components/page-header"
import { IpLink } from "@/components/ip-link"
import { UserLink } from "@/components/user-link"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { assignSession, endSession, listAccounts, listResellers, listSessions, listTools, type Reseller } from "@/lib/api/client"
import type { LiveSession } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  assigned_account_id: z.string().min(1, "Choose an account"),
})

type FormValues = z.infer<typeof schema>

export default function SessionsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [toolId, setToolId] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${toolId}-${resellerId}`,
    async () => {
      const [sessions, tools, resellers] = await Promise.all([
        listSessions({
          page,
          pageSize,
          query,
          toolId: toolId === "all" ? 0 : Number(toolId),
          resellerId: session?.role === "master" && resellerId !== "all" ? Number(resellerId) : 0,
        }),
        listTools(),
        session?.role === "master"
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return {
        sessions,
        tools,
        resellers: resellers.items,
      }
    },
  )
  const [ending, setEnding] = useState<LiveSession | null>(null)
  const [reassign, setReassign] = useState<LiveSession | null>(null)
  const [pending, setPending] = useState(false)

  const rows = data?.sessions.items ?? []

  const columns: DataColumn<LiveSession>[] = [
    {
      id: "username",
      header: "Username",
      cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
      sortValue: (row) => row.username,
    },
    { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
    { id: "ip", header: "Client IP", cell: (row) => <IpLink ip={row.client_ip} />, sortValue: (row) => row.client_ip },
    { id: "account", header: "Assigned account", cell: (row) => row.assigned_account_name, sortValue: (row) => row.assigned_account_name },
    { id: "created", header: "Created", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
    { id: "expires", header: "Expires", cell: (row) => formatTime(row.expires_at), sortValue: (row) => row.expires_at },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-3">
          <Button variant="outline" size="sm" onClick={() => setReassign(row)}>
            Assign
          </Button>
          <Button variant="destructive" size="sm" onClick={() => setEnding(row)}>
            End
          </Button>
        </div>
      ),
    },
  ]

  return (
    <>
      <PageHeader title="Active Logins" description="Live sessions. Assign a mapped account or end the login." />
      <ListFilters
        query={query}
        onQuery={(value) => {
          setQuery(value)
          setPage(1)
        }}
        searchId="session-search"
        placeholder="Username or IP"
        toolId={toolId}
        onTool={(value) => {
          setToolId(value)
          setPage(1)
        }}
        tools={data?.tools ?? []}
        resellerId={session?.role === "master" ? resellerId : undefined}
        onReseller={
          session?.role === "master"
            ? (value) => {
                setResellerId(value)
                setPage(1)
              }
            : undefined
        }
        resellers={session?.role === "master" ? data?.resellers : undefined}
      />
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            paging={{
              page: data?.sessions.page ?? page,
              total: data?.sessions.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={`${query}-${toolId}-${resellerId}`}
            emptyTitle={query ? "No matches" : "No active logins"}
            emptyDescription="Sessions appear here while they are still valid."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={Boolean(reassign)}
        onOpenChange={(next) => { if (!next) setReassign(null) }}
        title="Reassign session"
        description="Pin this login to a mapped account, or send it back to auto-switch."
      >
        {reassign ? (
          <ReassignForm
            key={reassign.id}
            session={reassign}
            onDone={async () => {
              setReassign(null)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <ConfirmDialog
        open={Boolean(ending)}
        onOpenChange={(next) => { if (!next) setEnding(null) }}
        title="Force logout"
        description={ending ? `End the session for ${ending.username} on ${ending.website_name}?` : ""}
        confirmLabel="End"
        pending={pending}
        onConfirm={() => {
          if (!ending) return
          setPending(true)
          void runMutation(() => endSession(ending.id), "Session ended").then((ok) => {
            setPending(false)
            if (ok) {
              setEnding(null)
              reload()
            }
          })
        }}
      />
    </>
  )
}

function ReassignForm({
  session,
  onDone,
}: {
  session: LiveSession
  onDone: () => Promise<void>
}) {
  const { data, loading } = useResource(`session-accounts-${session.website_id}`, () =>
    listAccounts({ page: 1, pageSize: 200, query: "", websiteId: session.website_id }),
  )
  const choices = data?.items ?? []
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { assigned_account_id: String(session.assigned_account_id) },
  })

  async function onSubmit(values: FormValues) {
    const ok = await runMutation(
      () => assignSession(session.id, Number(values.assigned_account_id)),
      "Session reassigned"
    )
    if (ok) await onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(form.formState.errors.assigned_account_id) || undefined}>
          <FieldLabel htmlFor="assigned_account_id">Assigned account</FieldLabel>
          <Controller
            control={form.control}
            name="assigned_account_id"
            render={({ field }) => (
              <ChoiceSelect
                id="assigned_account_id"
                value={field.value}
                onValueChange={field.onChange}
                items={[
                  { label: "Auto-switch", value: "0" },
                  ...choices.map((account) => ({
                    label: `${account.name} (${account.status})`,
                    value: String(account.id),
                  })),
                ]}
              />
            )}
          />
          <FieldError errors={[form.formState.errors.assigned_account_id]} />
          {loading ? <Spinner /> : null}
          {!loading && choices.length === 0 ? (
            <p className="text-sm text-muted-foreground">No mapped accounts for this website.</p>
          ) : null}
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save assignment
        </Button>
      </FieldGroup>
    </form>
  )
}
