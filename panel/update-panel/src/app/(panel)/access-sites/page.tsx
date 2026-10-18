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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field"
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
  const [deleting, setDeleting] = useState(false)

  const rows = data ?? []
  const columns: DataColumn<AccessSite>[] = [
    { id: "domain", header: "Website", cell: (row) => row.domain, sortValue: (row) => row.domain },
    {
      id: "tools",
      header: "Tools",
      cell: (row) => {
        const names = (row.tools ?? []).map((tool) => tool.access_slug || tool.name).filter(Boolean)
        return names.length ? names.join(", ") : "None"
      },
      sortValue: (row) => (row.tools ?? []).map((tool) => tool.name).join(", "),
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
  const [websiteQuery, setWebsiteQuery] = useState("")
  const shown = websites.filter((website) => {
    const needle = websiteQuery.trim().toLowerCase()
    if (!needle) return true
    return website.name.toLowerCase().includes(needle) || website.domain.toLowerCase().includes(needle) || (website.access_slug ?? "").includes(needle)
  })

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
        <FieldSet>
          <FieldLegend variant="label">Tools</FieldLegend>
          <FieldDescription>This site can open only the tools you check. {selected.length} selected.</FieldDescription>
          <Input
            value={websiteQuery}
            placeholder="Search tools"
            onChange={(event) => setWebsiteQuery(event.target.value)}
          />
          <FieldGroup className="max-h-72 overflow-y-auto">
            {shown.map((website) => {
              const checked = selected.includes(website.id)
              const button = website.access_slug ? website.access_slug : "no button"
              return (
                <Field key={website.id} orientation="horizontal">
                  <Checkbox
                    id={`tool-${website.id}`}
                    checked={checked}
                    onCheckedChange={(next) => {
                      const current = form.getValues("website_ids")
                      choose(next ? [...current, website.id] : current.filter((id) => id !== website.id))
                    }}
                  />
                  <FieldLabel htmlFor={`tool-${website.id}`}>{website.name} · {button}</FieldLabel>
                </Field>
              )
            })}
            {shown.length === 0 ? <p className="text-sm text-muted-foreground">No tool matches.</p> : null}
          </FieldGroup>
        </FieldSet>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          {site ? "Save tools" : "Create key"}
        </Button>
      </FieldGroup>
    </form>
  )
}
