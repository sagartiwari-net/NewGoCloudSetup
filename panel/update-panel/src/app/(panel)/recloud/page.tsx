"use client"

import { useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

import { useAuth } from "@/components/auth-provider"
import { CookieSourceField } from "@/components/cookie-source-field"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { SecretTextarea } from "@/components/secret-field"
import { Truncated } from "@/components/truncated"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { getRecloud, listRecloud, saveRecloud } from "@/lib/api/client"
import type { RecloudTool, RecloudToolDetail } from "@/lib/api/types"
import { formatIst } from "@/lib/format"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  upstream_url: z
    .string()
    .trim()
    .refine((value) => {
      try {
        const url = new URL(value)
        return (url.protocol === "http:" || url.protocol === "https:") && url.username === "" && url.password === ""
      } catch {
        return false
      }
    }, "Enter an http or https upstream URL"),
  cookie: z.string().trim().min(1, "Enter a cookie"),
})

type FormValues = z.infer<typeof schema>

export default function RecloudPage() {
  const { session } = useAuth()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<number | null>(null)
  const { data, loading, reload } = useResource(session?.role ?? "none", listRecloud)
  const rows = data ?? []

  const columns: DataColumn<RecloudTool>[] = [
    {
      id: "tool",
      header: "Tool",
      cell: (row) => (
        <div className="flex min-w-0 flex-col">
          <span>{row.tool_name}</span>
          <span className="text-sm text-muted-foreground">{row.domain}</span>
        </div>
      ),
      sortValue: (row) => row.tool_name,
    },
    {
      id: "url",
      header: "Upstream URL",
      cell: (row) => <Truncated value={row.upstream_url || "—"} />,
      sortValue: (row) => row.upstream_url,
    },
    {
      id: "cookie",
      header: "Cookie updated",
      cell: (row) => (row.cookie_updated_at ? formatIst(row.cookie_updated_at) : "—"),
      sortValue: (row) => row.cookie_updated_at,
    },
    {
      id: "edit",
      header: "Update",
      cell: (row) => (
        <Button
          variant="outline"
          size="sm"
          onClick={() => {
            setEditing(row.website_id)
            setOpen(true)
          }}
        >
          Update
        </Button>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Recloud"
        description="Cookie and upstream URL for recloud tools. A save applies on the next request."
      />
      <Card>
        <CardContent>
          <DataTable
            rows={rows}
            columns={columns}
            rowKey={(row) => row.website_id}
            loading={loading}
            emptyTitle="No recloud tools"
            emptyDescription="A tool shows up here after it is added as recloud."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title="Update recloud tool"
        description="Save the cookie and the group-buy site URL. The running tool reads both on the next request."
      >
        {open && editing ? (
          <RecloudEditor
            key={editing}
            websiteId={editing}
            onDone={() => {
              setOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
    </>
  )
}

function RecloudEditor({ websiteId, onDone }: { websiteId: number; onDone: () => void }) {
  const { data, loading } = useResource(`recloud-${websiteId}`, () => getRecloud(websiteId))
  if (loading || !data) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner />
        Loading tool…
      </div>
    )
  }
  return <RecloudForm tool={data} onDone={onDone} />
}

function RecloudForm({ tool, onDone }: { tool: RecloudToolDetail; onDone: () => void }) {
  const { session } = useAuth()
  const [saving, setSaving] = useState(false)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      upstream_url: tool.upstream_url,
      cookie: tool.cookie,
    },
  })
  const errors = form.formState.errors

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={form.handleSubmit((values) => {
        setSaving(true)
        void runMutation(
          () =>
            saveRecloud({
              website_id: tool.website_id,
              upstream_url: values.upstream_url.trim(),
              cookie: values.cookie.trim(),
            }),
          "Saved. The next request uses this cookie and URL.",
        ).then((ok) => {
          setSaving(false)
          if (ok) onDone()
        })
      })}
    >
      <FieldGroup>
        <Field>
          <FieldLabel>Tool</FieldLabel>
          <Input value={tool.tool_name} readOnly />
        </Field>
        <Field data-invalid={Boolean(errors.upstream_url) || undefined}>
          <FieldLabel htmlFor="upstream_url">Upstream URL</FieldLabel>
          <Input id="upstream_url" placeholder="https://example.com" {...form.register("upstream_url")} />
          <FieldError errors={[errors.upstream_url]} />
        </Field>
        {session?.role === "master" && tool.account_id > 0 ? (
          <CookieSourceField
            accountId={tool.account_id}
            onApply={(copied) => {
              form.setValue("cookie", copied.cookie, { shouldValidate: true })
            }}
          />
        ) : null}
        <Field data-invalid={Boolean(errors.cookie) || undefined}>
          <FieldLabel htmlFor="cookie">Cookie</FieldLabel>
          <Controller
            control={form.control}
            name="cookie"
            render={({ field }) => (
              <SecretTextarea id="cookie" value={field.value} onChange={field.onChange} invalid={Boolean(errors.cookie)} />
            )}
          />
          <FieldError errors={[errors.cookie]} />
        </Field>
      </FieldGroup>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button type="button" variant="outline" onClick={onDone}>
          Cancel
        </Button>
        <Button type="submit" disabled={saving}>
          {saving ? <Spinner data-icon="inline-start" /> : null}
          Save
        </Button>
      </div>
    </form>
  )
}
