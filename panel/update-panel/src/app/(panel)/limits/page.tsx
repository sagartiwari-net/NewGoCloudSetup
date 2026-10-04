"use client"

import { useMemo, useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon } from "lucide-react"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { ListFilters } from "@/components/list-filters"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { UserLink } from "@/components/user-link"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Toggle } from "@/components/ui/toggle"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useResource } from "@/hooks/use-resource"
import {
  deleteUser,
  listResellers,
  listTools,
  listUsers,
  listAllWebsites,
  resetUserUsage,
  saveUser,
  type Reseller,
} from "@/lib/api/client"
import type { PanelUser, Tool, Website } from "@/lib/api/types"
import { formatLimit, formatMeterList, formatReset, limitHint } from "@/lib/format"
import { useListPage } from "@/lib/page-size"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  username: z.string().trim().min(1, "Enter a username"),
  website_id: z.string().min(1, "Choose a website"),
  status: z.enum(["active", "suspended"]),
  custom_limit_expire_at: z.string().refine((value) => value === "" || /^\d{4}-\d{2}-\d{2}$/.test(value), "Use YYYY-MM-DD"),
  limit_visibility: z.enum(["account", "show", "hide"]),
})

type FormValues = z.infer<typeof schema>

export default function LimitsPage() {
  const { session } = useAuth()
  const [tab, setTab] = useState("all")
  const [query, setQuery] = useState("")
  const [toolId, setToolId] = useState("all")
  const [resellerId, setResellerId] = useState("all")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-${page}-${pageSize}-${query}-${toolId}-${resellerId}-${tab}`,
    async () => {
      const [users, websites, tools, resellers] = await Promise.all([
        listUsers({
          page,
          pageSize,
          query,
          toolId: toolId === "all" ? 0 : Number(toolId),
          resellerId: session?.role === "master" && resellerId !== "all" ? Number(resellerId) : 0,
          custom: tab === "custom",
        }),
        listAllWebsites(),
        listTools(),
        session?.role === "master"
          ? listResellers({ page: 1, pageSize: 50, query: "", status: "all" })
          : Promise.resolve({ items: [] as Reseller[], total: 0, page: 1, pageSize: 50 }),
      ])
      return { users, websites, tools, resellers: resellers.items }
    },
  )
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<PanelUser | null>(null)
  const [pendingDelete, setPendingDelete] = useState<PanelUser | null>(null)
  const [deleting, setDeleting] = useState(false)

  const limitedTools = (data?.tools ?? []).filter((tool) => Array.isArray(tool.limits) && tool.limits.length > 0)
  const limitedWebsites = (data?.websites ?? []).filter((website) =>
    limitedTools.some((tool) => tool.id === website.tool_id),
  )

  const resellerByWebsite = useMemo(() => {
    const map = new Map<number, string>()
    for (const reseller of data?.resellers ?? []) {
      for (const websiteId of reseller.website_ids) {
        if (!map.has(websiteId)) map.set(websiteId, reseller.username)
      }
    }
    return map
  }, [data?.resellers])

  const rows = data?.users.items ?? []
  const isMaster = session?.role === "master"

  const columns: DataColumn<PanelUser>[] = [
    {
      id: "username",
      header: "Username",
      cell: (row) => <UserLink username={row.username} websiteId={row.website_id} />,
      sortValue: (row) => row.username,
    },
    { id: "tool", header: "Tool", cell: (row) => row.tool_name, sortValue: (row) => row.tool_name },
    { id: "website", header: "Website", cell: (row) => row.website_name, sortValue: (row) => row.website_name },
    ...(isMaster
      ? [
          {
            id: "reseller",
            header: "Reseller",
            cell: (row: PanelUser) => resellerByWebsite.get(row.website_id) ?? "—",
            sortValue: (row: PanelUser) => resellerByWebsite.get(row.website_id) ?? "",
          } satisfies DataColumn<PanelUser>,
        ]
      : []),
    { id: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} />, sortValue: (row) => row.status },
    {
      id: "meters",
      header: "Limits",
      cell: (row) => formatMeterList(row.meters),
      sortValue: (row) => row.meters[0]?.used ?? 0,
    },
    {
      id: "limit_icon",
      header: "Limit icon",
      cell: (row) => (row.limit_visibility === "show" ? "Show" : row.limit_visibility === "hide" ? "Hide" : "Default"),
      sortValue: (row) => row.limit_visibility,
    },
    {
      id: "expiry",
      header: "Expiry",
      cell: (row) => row.custom_limit_expire_at ?? "—",
      sortValue: (row) => row.custom_limit_expire_at ?? "",
    },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-3">
          <Button variant="outline" size="sm" onClick={() => { setEditing(row); setOpen(true) }}>
            Edit
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void runMutation(() => resetUserUsage(row.id), "Usage reset").then((ok) => {
                if (ok) reload()
              })
            }}
          >
            Reset
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
        title="Limits"
        description="Only tools that have limits. Create and edit per-website meters here."
        action={
          <Button onClick={() => { setEditing(null); setOpen(true) }}>
            <PlusIcon data-icon="inline-start" />
            New limit
          </Button>
        }
      />
      <Tabs
        value={tab}
        onValueChange={(value) => {
          setTab(value)
          setPage(1)
        }}
      >
        <TabsList>
          <TabsTrigger value="all">All limits</TabsTrigger>
          <TabsTrigger value="custom">Custom limits</TabsTrigger>
        </TabsList>
        <TabsContent value={tab} className="flex flex-col gap-4">
          <ListFilters
            query={query}
            onQuery={(value) => {
              setQuery(value)
              setPage(1)
            }}
            searchId="limit-search"
            placeholder="Username, tool, or website"
            toolId={toolId}
            onTool={(value) => {
              setToolId(value)
              setPage(1)
            }}
            tools={limitedTools}
            resellerId={isMaster ? resellerId : undefined}
            onReseller={
              isMaster
                ? (value) => {
                    setResellerId(value)
                    setPage(1)
                  }
                : undefined
            }
            resellers={isMaster ? data?.resellers : undefined}
          />
          <Card>
            <CardContent>
              <DataTable
                rows={rows}
                columns={columns}
                rowKey={(row) => row.id}
                loading={loading}
                paging={{
                  page: data?.users.page ?? page,
                  total: data?.users.total ?? 0,
                  pageSize,
                  onPageChange: setPage,
                }}
                resetKey={`${tab}-${query}-${toolId}-${resellerId}`}
                emptyTitle={query || toolId !== "all" || resellerId !== "all" ? "No matches" : "No limits"}
                emptyDescription="Add a username on a website whose tool has at least one limit."
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit user limit" : "New user limit"}
        description="The meters shown match the website's tool."
      >
        {open ? (
          <UserForm
            key={editing?.id ?? "new"}
            user={editing}
            websites={limitedWebsites}
            tools={data?.tools ?? []}
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
        title="Delete user limit"
        description={pendingDelete ? `Delete the limit for ${pendingDelete.username}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteUser(pendingDelete.id), "User deleted").then((ok) => {
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

function UserForm({
  user,
  websites,
  tools,
  onDone,
}: {
  user: PanelUser | null
  websites: Website[]
  tools: Tool[]
  onDone: () => Promise<void>
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      username: user?.username ?? "",
      website_id: user ? String(user.website_id) : websites[0] ? String(websites[0].id) : "",
      status: user?.status ?? "active",
      custom_limit_expire_at: user?.custom_limit_expire_at ?? "",
      limit_visibility: user?.limit_visibility === "show" || user?.limit_visibility === "hide" ? user.limit_visibility : "account",
    },
  })
  const websiteId = form.watch("website_id")
  const website = websites.find((row) => String(row.id) === websiteId)
  const tool = tools.find((row) => row.id === website?.tool_id)
  const [limits, setLimits] = useState<Record<string, number>>(() => {
    const initial: Record<string, number> = {}
    for (const item of user?.meters ?? []) initial[item.key] = item.limit
    return initial
  })

  function limitsFor(nextWebsiteId: string) {
    const nextWebsite = websites.find((row) => String(row.id) === nextWebsiteId)
    const nextTool = tools.find((row) => row.id === nextWebsite?.tool_id)
    const next: Record<string, number> = {}
    for (const definition of nextTool?.limits ?? []) {
      const existing = user && user.website_id === nextWebsite?.id
        ? user.meters.find((item) => item.key === definition.key)?.limit
        : undefined
      next[definition.key] = existing ?? nextWebsite?.default_limits[definition.key] ?? -1
    }
    return next
  }

  async function onSubmit(values: FormValues) {
    const chosen = tools.find((row) => row.id === websites.find((site) => String(site.id) === values.website_id)?.tool_id)
    if (!chosen || chosen.limits.length === 0) return
    const chosenWebsite = websites.find((site) => String(site.id) === values.website_id)
    const meters = chosen.limits.map((definition) => ({
      key: definition.key,
      limit: limits[definition.key] ?? chosenWebsite?.default_limits[definition.key] ?? -1,
      used: user?.meters.find((item) => item.key === definition.key)?.used ?? 0,
    }))
    const ok = await runMutation(
      () =>
        saveUser({
          id: user?.id,
          username: values.username,
          website_id: Number(values.website_id),
          status: values.status,
          custom_limit_expire_at: values.custom_limit_expire_at || null,
          limit_visibility: values.limit_visibility === "account" ? "" : values.limit_visibility,
          meters,
        }),
      user ? "User updated" : "User saved",
    )
    if (ok) await onDone()
  }

  const errors = form.formState.errors

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.username) || undefined}>
          <FieldLabel htmlFor="username">Username</FieldLabel>
          <Input id="username" aria-invalid={Boolean(errors.username) || undefined} {...form.register("username")} />
          <FieldError errors={[errors.username]} />
        </Field>
        <Field data-invalid={Boolean(errors.website_id) || undefined}>
          <FieldLabel htmlFor="website_id">Website</FieldLabel>
          <Controller
            control={form.control}
            name="website_id"
            render={({ field }) => (
              <ChoiceSelect
                id="website_id"
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value)
                  setLimits(limitsFor(value))
                }}
                items={websites.map((site) => ({ label: site.name, value: String(site.id) }))}
              />
            )}
          />
          <FieldError errors={[errors.website_id]} />
        </Field>
        {(Array.isArray(tool?.limits) ? tool.limits : []).map((definition) => (
          <Field key={definition.key}>
            <FieldLabel htmlFor={`limit-${definition.key}`}>
              {definition.label} limit
            </FieldLabel>
            <Input
              id={`limit-${definition.key}`}
              type="number"
              min={-1}
              value={limits[definition.key] ?? website?.default_limits[definition.key] ?? -1}
              onChange={(event) =>
                setLimits((current) => ({ ...current, [definition.key]: Number(event.target.value) }))
              }
            />
            <FieldDescription>
              {formatReset(definition.reset_days)}. Used {user?.meters.find((item) => item.key === definition.key)?.used ?? 0}. Website default {formatLimit(website?.default_limits[definition.key] ?? -1)}. {limitHint(limits[definition.key] ?? website?.default_limits[definition.key] ?? -1)}
            </FieldDescription>
          </Field>
        ))}
        <Field>
          <FieldLabel>Limit icon</FieldLabel>
          <Controller
            control={form.control}
            name="limit_visibility"
            render={({ field }) => (
              <div className="flex items-center gap-2">
                {([
                  ["account", "Default", "positive"],
                  ["show", "Show", "positive"],
                  ["hide", "Hide", "negative"],
                ] as const).map(([value, label, tone]) => (
                  <Toggle
                    key={value}
                    type="button"
                    pressed={field.value === value}
                    onPressedChange={() => field.onChange(value)}
                    data-tone={tone}
                  >
                    {label}
                  </Toggle>
                ))}
              </div>
            )}
          />
          <FieldDescription>Default follows that account&apos;s Show limit switch. Show or Hide applies only to this user.</FieldDescription>
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
                  if (next === "active" || next === "suspended") field.onChange(next)
                }}
              >
                <ToggleGroupItem value="active" data-tone="positive">Active</ToggleGroupItem>
                <ToggleGroupItem value="suspended" data-tone="negative">Suspended</ToggleGroupItem>
              </ToggleGroup>
            )}
          />
        </Field>
        <Field data-invalid={Boolean(errors.custom_limit_expire_at) || undefined}>
          <FieldLabel htmlFor="custom_limit_expire_at">Custom limit expiry</FieldLabel>
          <Input id="custom_limit_expire_at" type="date" aria-invalid={Boolean(errors.custom_limit_expire_at) || undefined} {...form.register("custom_limit_expire_at")} />
          <FieldDescription>Optional. Format YYYY-MM-DD. Setting a date marks this user as custom.</FieldDescription>
          <FieldError errors={[errors.custom_limit_expire_at]} />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save user
        </Button>
      </FieldGroup>
    </form>
  )
}
