"use client"

import { useMemo, useState } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon } from "lucide-react"

import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import {
  automationOptions,
  deleteAutomationCredential,
  listAutomationCredentials,
  saveAutomationCredential,
} from "@/lib/api/client"
import type { AutomationCredential, AutomationProvider } from "@/lib/api/types"
import { formatIst } from "@/lib/format"
import { runMutation } from "@/lib/mutate"
import { useListPage } from "@/lib/page-size"

function credentialSchema(creating: boolean) {
  return z
    .object({
      name: z.string().trim().min(1, "Enter a name"),
      provider: z.string().min(1, "Choose a provider"),
      username: z.string().trim().min(1, "Enter a username"),
      password: z.string(),
    })
    .superRefine((value, ctx) => {
      if (creating && value.password.trim() === "") {
        ctx.addIssue({ code: "custom", path: ["password"], message: "Enter a password" })
      }
    })
}

type FormValues = z.infer<ReturnType<typeof credentialSchema>>

function providerLabel(providers: AutomationProvider[], id: string) {
  return providers.find((item) => item.id === id)?.label ?? id
}

export default function AutomationCredentialsPage() {
  const [query, setQuery] = useState("")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(`automation-credentials-${page}-${pageSize}-${query}`, async () => {
    const [credentials, options] = await Promise.all([
      listAutomationCredentials({ page, pageSize, query }),
      automationOptions(),
    ])
    return { credentials, providers: options.providers }
  })
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AutomationCredential | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AutomationCredential | null>(null)
  const [deleting, setDeleting] = useState(false)
  const rows = data?.credentials.items ?? []
  const providers = data?.providers ?? []

  const columns: DataColumn<AutomationCredential>[] = [
    { id: "name", header: "Name", cell: (row) => row.name, sortValue: (row) => row.name },
    {
      id: "provider",
      header: "Provider",
      cell: (row) => providerLabel(providers, row.provider),
      sortValue: (row) => row.provider,
    },
    { id: "username", header: "Username", cell: (row) => row.username, sortValue: (row) => row.username },
    {
      id: "password",
      header: "Password",
      cell: (row) => (row.has_password ? "Saved" : "Missing"),
      sortValue: (row) => (row.has_password ? 1 : 0),
    },
    {
      id: "updated",
      header: "Updated",
      cell: (row) => (row.updated_at ? formatIst(row.updated_at) : "—"),
      sortValue: (row) => row.updated_at,
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
        title="Credentials"
        description="Portal logins the worker can reuse. The password is encrypted and is not shown again."
        action={
          <Button
            onClick={() => {
              setEditing(null)
              setOpen(true)
            }}
          >
            <PlusIcon data-icon="inline-start" />
            New credential
          </Button>
        }
      />
      <Field className="md:max-w-sm">
        <FieldLabel htmlFor="credential-search">Search</FieldLabel>
        <Input
          id="credential-search"
          value={query}
          placeholder="Name, provider, or username"
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
            paging={{
              page: data?.credentials.page ?? page,
              total: data?.credentials.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={query}
            emptyTitle={query ? "No matches" : "No credentials"}
            emptyDescription="Add a portal login, then point jobs at it."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit credential" : "New credential"}
        description="One credential is shared by every job for that provider."
      >
        {open ? (
          <CredentialForm
            key={editing?.id ?? "new"}
            credential={editing}
            providers={providers}
            onDone={() => {
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
        title="Delete credential"
        description={pendingDelete ? `Delete ${pendingDelete.name}? Jobs that still use it will block this.` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteAutomationCredential(pendingDelete.id), "Credential deleted").then((ok) => {
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

function CredentialForm({
  credential,
  providers,
  onDone,
}: {
  credential: AutomationCredential | null
  providers: AutomationProvider[]
  onDone: () => void
}) {
  const schema = useMemo(() => credentialSchema(!credential), [credential])
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: credential?.name ?? "",
      provider: credential?.provider ?? providers[0]?.id ?? "",
      username: credential?.username ?? "",
      password: "",
    },
  })
  const errors = form.formState.errors

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={form.handleSubmit(async (values) => {
        const ok = await runMutation(
          () =>
            saveAutomationCredential({
              id: credential?.id,
              name: values.name.trim(),
              provider: values.provider,
              username: values.username.trim(),
              password: values.password,
            }),
          "Credential saved",
        )
        if (ok) onDone()
      })}
    >
      <FieldGroup>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="credential-name">Name</FieldLabel>
          <Input id="credential-name" placeholder="Markhor" {...form.register("name")} />
          <FieldError errors={[errors.name]} />
        </Field>
        <Field data-invalid={Boolean(errors.provider) || undefined}>
          <FieldLabel htmlFor="credential-provider">Provider</FieldLabel>
          <ChoiceSelect
            id="credential-provider"
            value={form.watch("provider")}
            onValueChange={(value) => form.setValue("provider", value, { shouldValidate: true })}
            items={providers.map((item) => ({ label: item.label, value: item.id }))}
            placeholder="Choose a provider"
            invalid={Boolean(errors.provider)}
          />
          <FieldError errors={[errors.provider]} />
        </Field>
        <Field data-invalid={Boolean(errors.username) || undefined}>
          <FieldLabel htmlFor="credential-username">Username</FieldLabel>
          <Input id="credential-username" autoComplete="off" {...form.register("username")} />
          <FieldError errors={[errors.username]} />
        </Field>
        <Field data-invalid={Boolean(errors.password) || undefined}>
          <FieldLabel htmlFor="credential-password">Password</FieldLabel>
          <Input id="credential-password" type="password" autoComplete="new-password" {...form.register("password")} />
          {credential ? <FieldDescription>Leave blank to keep the current password.</FieldDescription> : null}
          <FieldError errors={[errors.password]} />
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
