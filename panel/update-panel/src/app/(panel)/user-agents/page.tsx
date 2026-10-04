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
import { Truncated } from "@/components/truncated"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { useResource } from "@/hooks/use-resource"
import { deleteUserAgent, listUserAgents, saveUserAgent } from "@/lib/api/client"
import type { UserAgentProfile } from "@/lib/api/types"
import { runMutation } from "@/lib/mutate"
import { useListPage } from "@/lib/page-size"

const schema = z.object({
  name: z.string().trim().min(1, "Enter a name"),
  user_agent: z.string().trim().min(1, "Enter a user agent"),
})

type FormValues = z.infer<typeof schema>

export default function UserAgentsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}`,
    () => listUserAgents({ page, pageSize, query }),
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<UserAgentProfile | null>(null)
  const [pendingDelete, setPendingDelete] = useState<UserAgentProfile | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.items ?? []

  const columns: DataColumn<UserAgentProfile>[] = [
    { id: "name", header: "Name", cell: (row) => row.name, sortValue: (row) => row.name },
    {
      id: "agent",
      header: "User agent",
      cell: (row) => <Truncated value={row.user_agent} />,
      sortValue: (row) => row.user_agent,
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
        title="User Agents"
        description="Saved browser strings. Accounts that pick one follow later edits."
        action={
          <Button onClick={() => { setEditing(null); setOpen(true) }}>
            <PlusIcon data-icon="inline-start" />
            New user agent
          </Button>
        }
      />
      <Field className="max-w-sm">
        <FieldLabel htmlFor="agent-search">Search</FieldLabel>
        <Input
          id="agent-search"
          value={query}
          placeholder="Name"
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
            rowKey={(row) => row.id}
            loading={loading}
            paging={{ page: data?.page ?? page, total: data?.total ?? 0, pageSize, onPageChange: setPage }}
            resetKey={query}
            emptyTitle={query ? "No matches" : "No saved user agents"}
            emptyDescription="Add a name and the full user-agent string."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit user agent" : "New user agent"}
        description="Changing the string updates every account that selected this saved agent."
      >
        {open ? (
          <AgentForm
            key={editing?.id ?? "new"}
            agent={editing}
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
        title="Delete user agent"
        description={pendingDelete ? `Delete ${pendingDelete.name}? Accounts using it keep the last string as a one-off.` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteUserAgent(pendingDelete.id), "User agent deleted").then((ok) => {
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

function AgentForm({ agent, onDone }: { agent: UserAgentProfile | null; onDone: () => Promise<void> }) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: agent?.name ?? "", user_agent: agent?.user_agent ?? "" },
  })
  const errors = form.formState.errors

  async function onSubmit(values: FormValues) {
    const ok = await runMutation(
      () => saveUserAgent({ id: agent?.id, ...values }),
      agent ? "User agent updated" : "User agent saved",
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
        <Field data-invalid={Boolean(errors.user_agent) || undefined}>
          <FieldLabel htmlFor="user_agent">User agent</FieldLabel>
          <Textarea id="user_agent" rows={4} aria-invalid={Boolean(errors.user_agent) || undefined} {...form.register("user_agent")} />
          <FieldError errors={[errors.user_agent]} />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save user agent
        </Button>
      </FieldGroup>
    </form>
  )
}
