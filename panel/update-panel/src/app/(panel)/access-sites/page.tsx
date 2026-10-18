"use client"

import { useState } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon } from "lucide-react"

import { AccessToolPicker } from "@/components/access-tool-picker"
import { ChoiceSelect } from "@/components/choice-select"
import { useAuth } from "@/components/auth-provider"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { deleteAccessSite, listAccessSites, listWebsiteChoices, saveAccessSite } from "@/lib/api/client"
import type { AccessSite, Website } from "@/lib/api/types"
import { formatTime } from "@/lib/format"

const schema = z.object({
  domain: z.string().trim().min(1, "Enter the aMember domain"),
  note: z.string().trim().max(200, "Use 200 characters or fewer"),
  website_ids: z.array(z.number()),
})

type FormValues = z.infer<typeof schema>

export default function AccessSitesPage() {
  const { session } = useAuth()
  const { data, loading, reload } = useResource(session?.role ?? "none", listAccessSites)
  const { data: websites } = useResource(`${session?.role ?? "none"}-websites`, listWebsiteChoices)
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AccessSite | null>(null)
  const [issuedKey, setIssuedKey] = useState("")
  const [pendingDelete, setPendingDelete] = useState<AccessSite | null>(null)
  const [viewing, setViewing] = useState<AccessSite | null>(null)
  const [deleting, setDeleting] = useState(false)

  const rows = data ?? []
  const columns: DataColumn<AccessSite>[] = [
    { id: "domain", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
    {
      id: "tools",
      header: "Tools",
      cell: (row) => {
        const count = row.website_ids?.length ?? row.tools?.length ?? 0
        return (
          <Button variant="outline" size="sm" onClick={() => setViewing(row)}>
            {count === 0 ? "None" : `${count} tools`}
          </Button>
        )
      },
      sortValue: (row) => row.website_ids?.length ?? row.tools?.length ?? 0,
    },
    { id: "note", header: "Note", cell: (row) => row.note || "—", sortValue: (row) => row.note },
    { id: "enabled", header: "Status", cell: (row) => (row.enabled ? "Allowed" : "Off"), sortValue: (row) => (row.enabled ? "1" : "0") },
    { id: "created", header: "Added", cell: (row) => formatTime(row.created_at), sortValue: (row) => row.created_at },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setEditing(row)
              setOpen(true)
            }}
          >
            Tools
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void saveAccessSite({ id: row.id, enabled: !row.enabled, note: row.note }).then(() => reload())
            }}
          >
            {row.enabled ? "Turn off" : "Turn on"}
          </Button>
          <Button variant="destructive" size="sm" onClick={() => setPendingDelete(row)}>
            Remove
          </Button>
        </div>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Allowed sites"
        description="Only these aMember websites can ask the panel to open a tool, and only the tools you check."
        action={
          <Button onClick={() => { setEditing(null); setOpen(true) }}>
            <PlusIcon data-icon="inline-start" />
            Add site
          </Button>
        }
      />
      {issuedKey ? (
        <Alert>
          <AlertTitle>Copy this key now</AlertTitle>
          <AlertDescription>
            Put it in gt4acc.php. It is not shown again.
            <span className="mt-2 block font-mono break-all">{issuedKey}</span>
          </AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.id}
            loading={loading}
            emptyTitle="No allowed sites"
            emptyDescription="Add an aMember domain and choose which tools it can open."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Tools for this site" : "Add site"}
        description={editing ? "Unchecked tools stay closed, even when the customer has a matching plan." : "The key is created here and shown once. A site with no tools checked cannot open anything."}
      >
        {open ? (
          <SiteForm
            key={editing?.id ?? "new"}
            site={editing}
            websites={websites ?? []}
            onDone={async (key) => {
              if (key) setIssuedKey(key)
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <SiteToolsDialog
        key={viewing?.id ?? "closed"}
        site={viewing}
        websites={websites ?? []}
        onOpenChange={(next) => { if (!next) setViewing(null) }}
      />
      <ConfirmDialog
        open={pendingDelete != null}
        onOpenChange={(next) => { if (!next) setPendingDelete(null) }}
        title="Remove this site?"
        description="Its buttons stop working until you add it again. The old key will not work."
        confirmLabel="Remove"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void deleteAccessSite(pendingDelete.id).finally(() => {
            setDeleting(false)
            setPendingDelete(null)
            reload()
          })
        }}
      />
    </>
  )
}

function SiteToolsDialog({
  site,
  websites,
  onOpenChange,
}: {
  site: AccessSite | null
  websites: Website[]
  onOpenChange: (open: boolean) => void
}) {
  const [filter, setFilter] = useState("allowed")
  const [query, setQuery] = useState("")
  const allowed = new Set(site?.website_ids ?? [])
  const needle = query.trim().toLowerCase()
  const rows = websites
    .map((website) => ({
      id: website.id,
      name: website.name,
      slug: website.access_slug ?? "",
      allowed: allowed.has(website.id),
    }))
    .filter((row) => {
      if (filter === "allowed" && !row.allowed) return false
      if (filter === "not" && row.allowed) return false
      if (!needle) return true
      return row.name.toLowerCase().includes(needle) || row.slug.toLowerCase().includes(needle)
    })
    .sort((a, b) => a.name.localeCompare(b.name))

  return (
    <Dialog open={site != null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{site?.domain ?? "Tools"}</DialogTitle>
          <DialogDescription>Allowed tools open from this website. The rest stay closed.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <ChoiceSelect
            id="site-tool-filter"
            value={filter}
            onValueChange={setFilter}
            items={[
              { label: "Allowed", value: "allowed" },
              { label: "Not allowed", value: "not" },
              { label: "All", value: "all" },
            ]}
          />
          <Input value={query} placeholder="Search tools" onChange={(event) => setQuery(event.target.value)} />
          <div className="max-h-80 overflow-y-auto">
            {rows.length === 0 ? (
              <p className="text-sm text-muted-foreground">No tool matches.</p>
            ) : (
              <ul className="flex flex-col gap-2">
                {rows.map((row) => (
                  <li key={row.id} className="flex items-center justify-between gap-3 text-sm">
                    <span className="min-w-0 truncate">{row.slug ? `${row.name} · ${row.slug}` : row.name}</span>
                    <span className="shrink-0 text-muted-foreground">{row.allowed ? "Allowed" : "Not allowed"}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function SiteForm({
  site,
  websites,
  onDone,
}: {
  site: AccessSite | null
  websites: Website[]
  onDone: (key?: string) => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      domain: site?.domain ?? "",
      note: site?.note ?? "",
      website_ids: site?.website_ids ?? [],
    },
  })
  const [selected, setSelected] = useState(site?.website_ids ?? [])

  function choose(websiteIds: number[]) {
    setSelected(websiteIds)
    form.setValue("website_ids", websiteIds, { shouldValidate: true })
  }

  return (
    <form
      onSubmit={form.handleSubmit(async (values) => {
        const saved = await saveAccessSite({
          id: site?.id,
          domain: values.domain,
          note: values.note,
          enabled: site ? site.enabled : true,
          website_ids: values.website_ids,
        })
        await onDone(saved.key)
      })}
    >
      <FieldGroup>
        <Field data-invalid={Boolean(form.formState.errors.domain) || undefined}>
          <FieldLabel htmlFor="domain">aMember domain</FieldLabel>
          <Input id="domain" placeholder="toolsmandi.com" {...form.register("domain")} />
          <FieldError errors={[form.formState.errors.domain]} />
        </Field>
        <Field data-invalid={Boolean(form.formState.errors.note) || undefined}>
          <FieldLabel htmlFor="note">Note</FieldLabel>
          <Input id="note" {...form.register("note")} />
          <FieldError errors={[form.formState.errors.note]} />
        </Field>
        <AccessToolPicker websites={websites} selected={selected} onChange={choose} />
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          {site ? "Save tools" : "Create key"}
        </Button>
      </FieldGroup>
    </form>
  )
}
