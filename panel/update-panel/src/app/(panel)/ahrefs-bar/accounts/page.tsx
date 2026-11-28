"use client"

import { useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { toast } from "sonner"
import { CopyIcon, PlusIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useResource } from "@/hooks/use-resource"
import {
  deleteAhrefsBarAccount,
  listAhrefsBarAccounts,
  saveAhrefsBarAccount,
} from "@/lib/api/client"
import type { AhrefsBarAccount } from "@/lib/api/types"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

/** Service-worker console: IndexedDB Auth/Token + ahUserData → panel paste JSON */
const EXTRACT_TOKEN_CONSOLE = `;(async () => {
  const idbGet = () => new Promise((resolve, reject) => {
    const open = indexedDB.open('Auth');
    open.onerror = () => reject(open.error);
    open.onsuccess = () => {
      const db = open.result;
      const req = db.transaction('Token', 'readonly').objectStore('Token').get('token');
      req.onerror = () => { db.close(); reject(req.error); };
      req.onsuccess = () => { const v = req.result; db.close(); resolve(v); };
    };
  });
  const storageGet = (keys) => new Promise((r) => chrome.storage.local.get(keys, r));
  const [tokenRow, st] = await Promise.all([
    idbGet(),
    storageGet(['ahUserData', 'ahIsAuth3']),
  ]);
  if (!tokenRow || !tokenRow.accessToken) {
    console.warn('No IndexedDB Auth/Token — log into Ahrefs toolbar first');
    return;
  }
  const user = st.ahUserData || {};
  const paste = {
    accessToken: tokenRow.accessToken,
    expiresIn: tokenRow.expiresIn,
    expiresAt: tokenRow.expiresAt,
    email: user.email || '',
    workspaceName: user.workspaceName || '',
    keywordsLimit: user.keywordsLimit,
    siteExplorerLimit: user.siteExplorerLimit,
    hasTrafficAddon: user.hasTrafficAddon,
    ahIsAuth3: st.ahIsAuth3 === true,
  };
  const text = JSON.stringify(paste, null, 2);
  globalThis.__ahrefsPanelPaste = text;
  console.log('——— COPY BELOW ———');
  console.log(text);
  console.log('——— END ———');
})();`

const schema = z.object({
  name: z.string().trim().min(1, "Enter a name"),
  access_token_json: z.string(),
  status: z.enum(["active", "disabled", "bad", "cooldown"]),
})

type FormValues = z.infer<typeof schema>

export default function AhrefsBarAccountsPage() {
  const { session } = useAuth()
  const [query, setQuery] = useState("")
  const [status, setStatus] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${status}`,
    () => listAhrefsBarAccounts({ page, pageSize, query, status }),
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AhrefsBarAccount | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AhrefsBarAccount | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data?.items ?? []

  const columns: DataColumn<AhrefsBarAccount>[] = [
    { id: "name", header: "Name", cell: (row) => row.name, sortValue: (row) => row.name },
    { id: "token", header: "Token", cell: (row) => row.token_mask || "—", sortValue: (row) => row.token_mask },
    { id: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} />, sortValue: (row) => row.status },
    { id: "error", header: "Last error", cell: (row) => row.last_error || "—", sortValue: (row) => row.last_error },
    { id: "used", header: "Last used", cell: (row) => row.last_used_at || "—", sortValue: (row) => row.last_used_at },
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
        title="Ahrefs Bar accounts"
        description="Token pool for the toolbar proxy. List shows a mask; paste a new token only when rotating."
        action={
          <Button
            onClick={() => {
              setEditing(null)
              setOpen(true)
            }}
          >
            <PlusIcon data-icon="inline-start" />
            Add account
          </Button>
        }
      />
      <TokenExtractHelp />
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <Field className="md:max-w-sm md:flex-1">
          <FieldLabel htmlFor="ab-acc-search">Search</FieldLabel>
          <Input
            id="ab-acc-search"
            value={query}
            placeholder="Name"
            onChange={(e) => {
              setQuery(e.target.value)
              setPage(1)
            }}
          />
        </Field>
        <Field className="md:w-64">
          <FieldLabel htmlFor="ab-acc-status">Status</FieldLabel>
          <ChoiceSelect
            id="ab-acc-status"
            value={status}
            onValueChange={(value) => {
              setStatus(value)
              setPage(1)
            }}
            items={[
              { label: "All", value: "all" },
              { label: "active", value: "active" },
              { label: "bad", value: "bad" },
              { label: "disabled", value: "disabled" },
              { label: "cooldown", value: "cooldown" },
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
            emptyTitle={query ? "No matches" : "No accounts"}
            emptyDescription="Add an Ahrefs access token to the pool."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit account" : "Add account"}
        description="Paste plain bearer or Ahrefs token JSON. Leave blank on edit to keep the current token."
      >
        {open ? (
          <AccountForm
            key={editing?.id ?? "new"}
            account={editing}
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
        title="Delete account"
        description={pendingDelete ? `Delete ${pendingDelete.name}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteAhrefsBarAccount(pendingDelete.id), "Account deleted").then((ok) => {
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

function TokenExtractHelp() {
  return (
    <Card size="sm">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3 space-y-0">
        <CardTitle>Get Ahrefs token</CardTitle>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            void navigator.clipboard.writeText(EXTRACT_TOKEN_CONSOLE).then(
              () => toast.success("Console command copied"),
              () => toast.error("Could not copy"),
            )
          }}
        >
          <CopyIcon data-icon="inline-start" />
          Copy console command
        </Button>
      </CardHeader>
      <CardContent>
        <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
          <li>Log into Ahrefs in the official SEO Toolbar.</li>
          <li>
            <span className="font-mono">chrome://extensions</span> → Ahrefs SEO Toolbar → Service worker.
          </li>
          <li>Paste the copied command → Enter.</li>
          <li>Copy the JSON printed between the lines.</li>
          <li>Add account → Name = email → Access token = that JSON.</li>
        </ol>
      </CardContent>
    </Card>
  )
}

function AccountForm({
  account,
  onDone,
}: {
  account: AhrefsBarAccount | null
  onDone: () => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: account?.name ?? "",
      access_token_json: "",
      status: (["active", "disabled", "bad", "cooldown"].includes(account?.status ?? "")
        ? account!.status
        : "active") as FormValues["status"],
    },
  })
  const errors = form.formState.errors

  async function onSubmit(values: FormValues) {
    if (!account && !values.access_token_json.trim()) {
      form.setError("access_token_json", { message: "Paste a token" })
      return
    }
    const ok = await runMutation(
      () =>
        saveAhrefsBarAccount({
          id: account?.id,
          name: values.name,
          status: values.status,
          access_token_json: values.access_token_json.trim() || undefined,
        }),
      account ? "Account updated" : "Account saved",
    )
    if (ok) await onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="ab-name">Name</FieldLabel>
          <Input id="ab-name" aria-invalid={Boolean(errors.name) || undefined} {...form.register("name")} />
          <FieldError errors={[errors.name]} />
        </Field>
        <Field data-invalid={Boolean(errors.access_token_json) || undefined}>
          <FieldLabel htmlFor="ab-token">Access token / JSON</FieldLabel>
          <Textarea
            id="ab-token"
            rows={5}
            className="font-mono text-xs"
            placeholder={account ? "Leave blank to keep current token" : "Paste token…"}
            aria-invalid={Boolean(errors.access_token_json) || undefined}
            {...form.register("access_token_json")}
          />
          <FieldDescription>
            {account?.token_mask
              ? `Current: ${account.token_mask}`
              : "Paste ahToken JSON or plain bearer from the extract snippet above."}
          </FieldDescription>
          <FieldError errors={[errors.access_token_json]} />
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
                  if (next === "active" || next === "disabled" || next === "bad" || next === "cooldown") {
                    field.onChange(next)
                  }
                }}
              >
                <ToggleGroupItem value="active" data-tone="positive">Active</ToggleGroupItem>
                <ToggleGroupItem value="disabled">Disabled</ToggleGroupItem>
                <ToggleGroupItem value="bad" data-tone="negative">Bad</ToggleGroupItem>
              </ToggleGroup>
            )}
          />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save account
        </Button>
      </FieldGroup>
    </form>
  )
}
