"use client"

import { useEffect, useState } from "react"
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
import { SecretTextarea } from "@/components/secret-field"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useResource } from "@/hooks/use-resource"
import {
  deleteWebsite,
  listTools,
  listWebsites,
  saveTool,
  saveWebsite,
} from "@/lib/api/client"
import type { Tool, Website } from "@/lib/api/types"
import { formatLimit, formatReset, limitHint } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const websiteSchema = z.object({
  tool_id: z.string().min(1, "Choose a tool"),
  name: z.string().trim().min(1, "Enter a name"),
  domain: z.string().trim().min(1, "Enter a domain"),
  secret_key: z.string().trim().min(1, "Enter a handshake secret"),
  session_duration: z.number().int().min(1, "Use at least 1 minute"),
  session_security_enabled: z.boolean(),
})

const toolSchema = z.object({
  name: z.string().trim().min(1, "Enter a tool name"),
  category: z.string().trim().min(1, "Enter a category"),
})

function limitKey(label: string) {
  const key = label.trim().toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_|_$/g, "")
  return key || "limit"
}

type WebsiteValues = z.infer<typeof websiteSchema>
type ToolValues = z.infer<typeof toolSchema>

export default function WebsitesPage() {
  const { session } = useAuth()
  const { page, setPage, pageSize } = useListPage()
  const [query, setQuery] = useState("")
  const { data, loading, reload } = useResource(`${session?.role ?? "none"}-${page}-${pageSize}-${query}`, async () => {
    const [websites, tools] = await Promise.all([
      listWebsites({ page, pageSize, query }),
      listTools(),
    ])
    return { websites, tools }
  })
  const [open, setOpen] = useState(false)
  const [toolOpen, setToolOpen] = useState(false)
  const [editingTool, setEditingTool] = useState<Tool | null>(null)
  const [editing, setEditing] = useState<Website | null>(null)
  const [pendingDelete, setPendingDelete] = useState<Website | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [pickedTool, setPickedTool] = useState("")

  const rows = data?.websites.items ?? []

  const toolName = (toolId: number) => data?.tools.find((tool) => tool.id === toolId)?.name ?? "—"

  const columns: DataColumn<Website>[] = [
    { id: "name", header: "Name", cell: (row) => row.name, sortValue: (row) => row.name },
    { id: "domain", header: "Domain", cell: (row) => row.domain, sortValue: (row) => row.domain },
    { id: "tool", header: "Tool", cell: (row) => toolName(row.tool_id), sortValue: (row) => toolName(row.tool_id) },
    {
      id: "security",
      header: "Security",
      cell: (row) => (
        <Badge variant={row.session_security_enabled ? "default" : "secondary"}>
          {row.session_security_enabled ? "on" : "off"}
        </Badge>
      ),
      sortValue: (row) => (row.session_security_enabled ? "on" : "off"),
    },
    {
      id: "limits",
      header: "Default limits",
      cell: (row) => {
        const tool = data?.tools.find((item) => item.id === row.tool_id)
        const toolLimits = Array.isArray(tool?.limits) ? tool.limits : []
        if (toolLimits.length === 0) return "None"
        return toolLimits
          .map((definition) => `${definition.label} ${formatLimit(row.default_limits[definition.key] ?? -1)}`)
          .join(" · ")
      },
      sortValue: (row) => Object.values(row.default_limits)[0] ?? 0,
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
        title="Website Domains"
        description="Each website is one public tool domain. Handshake secrets stay masked in this form until revealed."
        action={
          <Button onClick={() => { setEditing(null); setOpen(true) }}>
            <PlusIcon data-icon="inline-start" />
            New website
          </Button>
        }
      />
      <Field className="max-w-sm">
        <FieldLabel htmlFor="website-search">Search</FieldLabel>
        <Input
          id="website-search"
          value={query}
          placeholder="Name or domain"
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
            resetKey={`${query}-${page}`}
            paging={{
              page: data?.websites.page ?? page,
              total: data?.websites.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            emptyTitle={query ? "No matches" : "No websites"}
            emptyDescription="Add a domain before mapping accounts onto it."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit website" : "New website"}
        description="The handshake secret is masked until you reveal it."
      >
        {open ? (
          <WebsiteForm
            key={editing?.id ?? "new"}
            website={editing}
            tools={data?.tools ?? []}
            preferredToolId={pickedTool}
            onAddTool={() => {
              setEditingTool(null)
              setToolOpen(true)
            }}
            onEditTool={(tool) => {
              setEditingTool(tool)
              setToolOpen(true)
            }}
            onDone={async () => {
              setOpen(false)
              setPickedTool("")
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <FormOverlay
        open={toolOpen}
        onOpenChange={(next) => {
          setToolOpen(next)
          if (!next) setEditingTool(null)
        }}
        title={editingTool ? "Edit tool" : "Add tool"}
        description={editingTool ? "Add or change this tool's limit meters. Existing meter keys stay put." : "Add a catalog entry, then attach a website domain to it."}
      >
        {toolOpen ? (
          <ToolForm
            key={editingTool?.id ?? "new-tool"}
            tool={editingTool}
            onDone={async (tool) => {
              setPickedTool(String(tool.id))
              setToolOpen(false)
              reload()
            }}
          />
        ) : null}
      </FormOverlay>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(next) => { if (!next) setPendingDelete(null) }}
        title="Delete website"
        description={pendingDelete ? `Delete ${pendingDelete.name}? Accounts on it must be removed first.` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteWebsite(pendingDelete.id), "Website deleted").then((ok) => {
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

function WebsiteForm({
  website,
  tools,
  preferredToolId,
  onAddTool,
  onEditTool,
  onDone,
}: {
  website: Website | null
  tools: Tool[]
  preferredToolId: string
  onAddTool: () => void
  onEditTool: (tool: Tool) => void
  onDone: () => Promise<void>
}) {
  const form = useForm<WebsiteValues>({
    resolver: zodResolver(websiteSchema),
    defaultValues: {
      tool_id: preferredToolId || (website ? String(website.tool_id) : tools[0] ? String(tools[0].id) : ""),
      name: website?.name ?? "",
      domain: website?.domain ?? "",
      secret_key: website?.secret_key ?? "",
      session_duration: website?.session_duration ?? 60,
      session_security_enabled: website?.session_security_enabled ?? true,
    },
  })
  const watchedToolId = form.watch("tool_id")
  const selectedTool = tools.find((tool) => String(tool.id) === watchedToolId)
  const [defaults, setDefaults] = useState<Record<string, number>>(website?.default_limits ?? {})

  useEffect(() => {
    if (preferredToolId) form.setValue("tool_id", preferredToolId)
  }, [form, preferredToolId])

  useEffect(() => {
    setDefaults((current) => {
      const next = { ...current }
      for (const definition of selectedTool?.limits ?? []) {
        if (next[definition.key] === undefined) next[definition.key] = website?.default_limits[definition.key] ?? -1
      }
      return next
    })
  }, [selectedTool, website])

  const errors = form.formState.errors

  async function onSubmit(values: WebsiteValues) {
    const ok = await runMutation(
      () =>
        saveWebsite({
          id: website?.id,
          tool_id: Number(values.tool_id),
          name: values.name,
          domain: values.domain,
          secret_key: values.secret_key,
          session_duration: values.session_duration,
          default_limits: Object.fromEntries(
            (selectedTool?.limits ?? []).map((definition) => [definition.key, defaults[definition.key] ?? -1]),
          ),
          session_security_enabled: values.session_security_enabled,
        }),
      website ? "Website updated" : "Website saved"
    )
    if (ok) await onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.tool_id) || undefined}>
          <FieldLabel htmlFor="tool_id">Tool</FieldLabel>
          <Controller
            control={form.control}
            name="tool_id"
            render={({ field }) => (
              <ChoiceSelect
                id="tool_id"
                value={field.value}
                onValueChange={field.onChange}
                items={tools.map((tool) => ({ label: tool.name, value: String(tool.id) }))}
              />
            )}
          />
          <div className="flex flex-wrap gap-3">
            <Button type="button" variant="outline" onClick={onAddTool}>
              <PlusIcon data-icon="inline-start" />
              Add tool to catalog
            </Button>
            {selectedTool ? (
              <Button type="button" variant="outline" onClick={() => onEditTool(selectedTool)}>
                Edit tool
              </Button>
            ) : null}
          </div>
          <FieldError errors={[errors.tool_id]} />
        </Field>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="name">Name</FieldLabel>
          <Input id="name" aria-invalid={Boolean(errors.name) || undefined} {...form.register("name")} />
          <FieldError errors={[errors.name]} />
        </Field>
        <Field data-invalid={Boolean(errors.domain) || undefined}>
          <FieldLabel htmlFor="domain">Domain</FieldLabel>
          <Input id="domain" aria-invalid={Boolean(errors.domain) || undefined} {...form.register("domain")} />
          <FieldError errors={[errors.domain]} />
        </Field>
        <Field data-invalid={Boolean(errors.secret_key) || undefined}>
          <FieldLabel htmlFor="secret_key">Handshake secret</FieldLabel>
          <Controller
            control={form.control}
            name="secret_key"
            render={({ field }) => (
              <SecretTextarea id="secret_key" value={field.value} onChange={field.onChange} invalid={Boolean(errors.secret_key)} />
            )}
          />
          <FieldError errors={[errors.secret_key]} />
        </Field>
        <Field data-invalid={Boolean(errors.session_duration) || undefined}>
          <FieldLabel htmlFor="session_duration">Session duration minutes</FieldLabel>
          <Input id="session_duration" type="number" aria-invalid={Boolean(errors.session_duration) || undefined} {...form.register("session_duration", { valueAsNumber: true })} />
          <FieldError errors={[errors.session_duration]} />
        </Field>
        {(selectedTool?.limits.length ?? 0) === 0 ? (
          <Field>
            <FieldLabel>Default limits</FieldLabel>
            <FieldDescription>This tool has no limits.</FieldDescription>
          </Field>
        ) : (
          selectedTool?.limits.map((definition) => (
            <Field key={definition.key}>
              <FieldLabel htmlFor={`default-${definition.key}`}>
                {definition.label} · {formatReset(definition.reset_days)}
              </FieldLabel>
              <Input
                id={`default-${definition.key}`}
                type="number"
                min={-1}
                value={defaults[definition.key] ?? -1}
                onChange={(event) =>
                  setDefaults((current) => ({ ...current, [definition.key]: Number(event.target.value) }))
                }
              />
              <FieldDescription>{limitHint(defaults[definition.key] ?? -1)}</FieldDescription>
            </Field>
          ))
        )}
        <Field orientation="horizontal">
          <Controller
            control={form.control}
            name="session_security_enabled"
            render={({ field }) => (
              <Switch id="session_security_enabled" checked={field.value} onCheckedChange={field.onChange} />
            )}
          />
          <FieldLabel htmlFor="session_security_enabled">Session security</FieldLabel>
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save website
        </Button>
      </FieldGroup>
    </form>
  )
}

type LimitRow = { key: string; label: string; reset_days: number }

function ToolForm({ tool, onDone }: { tool: Tool | null; onDone: (tool: Tool) => Promise<void> }) {
  const form = useForm<ToolValues>({
    resolver: zodResolver(toolSchema),
    defaultValues: { name: tool?.name ?? "", category: tool?.category ?? "" },
  })
  const [limits, setLimits] = useState<LimitRow[]>(
    tool && Array.isArray(tool.limits)
      ? tool.limits.map((item) => ({ key: item.key, label: item.label, reset_days: item.reset_days }))
      : [{ key: "", label: "Credits", reset_days: 1 }],
  )
  const errors = form.formState.errors

  async function onSubmit(values: ToolValues) {
    if (limits.some((item) => !item.label.trim())) {
      const { toast } = await import("sonner")
      toast.error("Each limit needs a label")
      return
    }
    if (limits.some((item) => !Number.isInteger(item.reset_days) || item.reset_days < 1 || item.reset_days > 365)) {
      const { toast } = await import("sonner")
      toast.error("Reset days must be from 1 to 365")
      return
    }
    try {
      const seen = new Set<string>()
      const toolSaved = await saveTool({
        id: tool?.id,
        ...values,
        limits: limits.map((item) => {
          let key = item.key || limitKey(item.label)
          while (seen.has(key)) key = `${key}_2`
          seen.add(key)
          return { key, label: item.label.trim(), reset_days: item.reset_days }
        }),
      })
      const { toast } = await import("sonner")
      toast.success(tool ? "Tool updated" : "Tool added")
      await onDone(toolSaved)
    } catch (error) {
      const { toast } = await import("sonner")
      toast.error(error instanceof Error ? error.message : "Could not save the tool")
    }
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="tool_name">Name</FieldLabel>
          <Input id="tool_name" aria-invalid={Boolean(errors.name) || undefined} {...form.register("name")} />
          <FieldError errors={[errors.name]} />
        </Field>
        <Field data-invalid={Boolean(errors.category) || undefined}>
          <FieldLabel htmlFor="category">Category</FieldLabel>
          <Input id="category" aria-invalid={Boolean(errors.category) || undefined} {...form.register("category")} />
          <FieldError errors={[errors.category]} />
        </Field>
        {limits.map((item, index) => (
          <Field key={index}>
            <FieldLabel htmlFor={`limit-label-${index}`}>Limit {index + 1} label</FieldLabel>
            <Input
              id={`limit-label-${index}`}
              value={item.label}
              onChange={(event) =>
                setLimits((current) => current.map((row, rowIndex) => (rowIndex === index ? { ...row, label: event.target.value } : row)))
              }
            />
            <FieldLabel htmlFor={`reset-days-${index}`}>Resets every (days)</FieldLabel>
            <Input
              id={`reset-days-${index}`}
              type="number"
              min={1}
              max={365}
              value={item.reset_days}
              onChange={(event) => {
                const next = Number(event.target.value)
                setLimits((current) => current.map((row, rowIndex) => (rowIndex === index ? { ...row, reset_days: next } : row)))
              }}
            />
            <ToggleGroup
              spacing={2}
              value={item.reset_days === 1 || item.reset_days === 7 || item.reset_days === 30 ? [String(item.reset_days)] : []}
              onValueChange={(values) => {
                const next = Number(values[values.length - 1])
                if (next === 1 || next === 7 || next === 30) {
                  setLimits((current) => current.map((row, rowIndex) => (rowIndex === index ? { ...row, reset_days: next } : row)))
                }
              }}
            >
              <ToggleGroupItem value="1">1 day</ToggleGroupItem>
              <ToggleGroupItem value="7">7 days</ToggleGroupItem>
              <ToggleGroupItem value="30">30 days</ToggleGroupItem>
            </ToggleGroup>
            <Button
              type="button"
              variant="outline"
              className="self-start"
              onClick={() => setLimits((current) => current.filter((_, rowIndex) => rowIndex !== index))}
            >
              Remove limit
            </Button>
          </Field>
        ))}
        {limits.length < 2 ? (
          <Button
            type="button"
            variant="outline"
            className="self-start"
            onClick={() =>
              setLimits((current) => [
                ...current,
                { key: `limit_${Date.now()}`, label: "", reset_days: 1 },
              ])
            }
          >
            Add limit
          </Button>
        ) : null}
        <FieldDescription>A tool can have no limits, one limit, or two. Clearing both is allowed.</FieldDescription>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save tool
        </Button>
      </FieldGroup>
    </form>
  )
}
