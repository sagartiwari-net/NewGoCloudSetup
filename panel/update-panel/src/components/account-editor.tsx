"use client"

import { useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { SecretTextarea } from "@/components/secret-field"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { deleteAccount, saveAccount } from "@/lib/api/client"
import type { IngestLog, MappedAccount, ProxyEndpoint, Role, UserAgentProfile, Website } from "@/lib/api/types"
import { formatTime } from "@/lib/format"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  website_id: z.string().min(1, "Choose a website"),
  name: z.string().trim().min(1, "Enter an account name"),
  description: z.string().max(280, "Use 280 characters or fewer"),
  cookie: z.string().trim().min(1, "Enter a cookie"),
  user_agent: z.string().trim().min(1, "Choose a user agent"),
  user_agent_id: z.string(),
  proxy_id: z.string(),
  proxy: z
    .string()
    .trim()
    .refine(
      (value) => value === "" || /^(socks5|https?):\/\//i.test(value),
      "Use a pool proxy or a socks5:// or http:// URL",
    ),
  status: z.enum(["active", "inactive", "logged_out", "blocked"]),
  show_limit: z.boolean(),
  automation_task_uid: z.string(),
  automation_ingest_key: z
    .string()
    .trim()
    .refine(
      (value) => value === "" || /^[a-z0-9_]+$/.test(value.toLowerCase()),
      "Use letters, numbers, and underscores",
    ),
})

type FormValues = z.infer<typeof schema>

function editorStatus(status: string): FormValues["status"] {
  if (status === "inactive" || status === "blocked") return status
  // logged_out (tool failover) → edit as active so a cookie paste can save cleanly
  return "active"
}

export function AccountEditor({
  account,
  websites,
  proxies,
  agents,
  latestIngest,
  role,
  onDone,
}: {
  account: MappedAccount | null
  websites: Website[]
  proxies: ProxyEndpoint[]
  agents: UserAgentProfile[]
  latestIngest: IngestLog | null
  role: Role
  onDone: () => void
}) {
  const [tab, setTab] = useState("basic")
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: account
      ? {
          website_id: String(account.website_id),
          name: account.name,
          description: account.description,
          cookie: account.cookie,
          user_agent: account.user_agent,
          user_agent_id: account.user_agent_id ? String(account.user_agent_id) : "",
          proxy: account.proxy,
          proxy_id: account.proxy_id ? String(account.proxy_id) : "",
          status: editorStatus(account.status),
          show_limit: account.show_limit,
          automation_task_uid: account.automation_task_uid,
          automation_ingest_key: account.automation_ingest_key,
        }
      : {
          website_id: websites[0] ? String(websites[0].id) : "",
          name: "",
          description: "",
          cookie: "",
          user_agent: agents[0]?.user_agent ?? "Mozilla/5.0 (compatible; ToolsMandiPanel/1.0)",
          user_agent_id: agents[0] ? String(agents[0].id) : "",
          proxy: "",
          proxy_id: "",
          status: "active",
          show_limit: true,
          automation_task_uid: "",
          automation_ingest_key: "",
        },
  })
  const errors = form.formState.errors

  async function onSubmit(values: FormValues) {
    // Cookie paste after tool failover must revive — never leave logged_out stuck.
    const nextStatus = values.cookie.trim() && values.status !== "inactive" && values.status !== "blocked" ? "active" : values.status
    const ok = await runMutation(
      () =>
        saveAccount({
          id: account?.id,
          website_id: Number(values.website_id),
          name: values.name,
          description: values.description.trim(),
          cookie: values.cookie,
          user_agent: values.user_agent,
          user_agent_id: values.user_agent_id ? Number(values.user_agent_id) : null,
          proxy: values.proxy,
          proxy_id: values.proxy_id ? Number(values.proxy_id) : null,
          status: nextStatus,
          show_limit: values.show_limit,
          automation_task_uid: role === "master" ? values.automation_task_uid.trim() : account?.automation_task_uid ?? "",
          automation_ingest_key:
            role === "master" ? values.automation_ingest_key.trim().toLowerCase() : account?.automation_ingest_key ?? "",
        }),
      account ? "Account updated" : "Account saved",
    )
    if (ok) onDone()
  }

  return (
    <>
      <form onSubmit={form.handleSubmit(onSubmit)}>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.name) || undefined}>
            <FieldLabel htmlFor="name">Account name</FieldLabel>
            <Input id="name" aria-invalid={Boolean(errors.name) || undefined} {...form.register("name")} />
            <FieldError errors={[errors.name]} />
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
                  onValueChange={field.onChange}
                  invalid={Boolean(errors.website_id)}
                  items={websites.map((website) => ({ label: website.name, value: String(website.id) }))}
                />
              )}
            />
            <FieldError errors={[errors.website_id]} />
          </Field>
        </FieldGroup>
        {role === "master" ? (
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList>
              <TabsTrigger value="basic">Basic</TabsTrigger>
              <TabsTrigger value="advanced">Advanced</TabsTrigger>
            </TabsList>
          </Tabs>
        ) : null}
        <div className={role === "reseller" || tab === "basic" ? "flex flex-col gap-4" : "hidden"}>
          <FieldGroup>
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
            <Field data-invalid={Boolean(errors.user_agent) || undefined}>
              <FieldLabel htmlFor="user_agent">User agent</FieldLabel>
              <Controller
                control={form.control}
                name="user_agent_id"
                render={({ field }) => (
                  <ChoiceSelect
                    id="user_agent"
                    value={field.value || "custom"}
                    invalid={Boolean(errors.user_agent)}
                    items={[
                      ...agents.map((agent) => ({ label: agent.name, value: String(agent.id) })),
                      { label: "One-off string", value: "custom" },
                    ]}
                    onValueChange={(next) => {
                      if (next === "custom") {
                        field.onChange("")
                        return
                      }
                      const agent = agents.find((item) => String(item.id) === next)
                      field.onChange(next)
                      if (agent) form.setValue("user_agent", agent.user_agent)
                    }}
                  />
                )}
              />
              {!form.watch("user_agent_id") ? (
                <Input aria-invalid={Boolean(errors.user_agent) || undefined} {...form.register("user_agent")} />
              ) : (
                <FieldDescription>{form.watch("user_agent")}</FieldDescription>
              )}
              <Button
                type="button"
                variant="outline"
                className="self-start"
                onClick={() => {
                  form.setValue("user_agent_id", "")
                  form.setValue("user_agent", navigator.userAgent)
                }}
              >
                Use this browser
              </Button>
              <FieldError errors={[errors.user_agent]} />
            </Field>
            <Field data-invalid={Boolean(errors.proxy) || undefined}>
              <FieldLabel htmlFor="proxy">Proxy</FieldLabel>
              <Controller
                control={form.control}
                name="proxy"
                render={({ field }) => (
                  <ProxyField
                    id="proxy"
                    value={field.value}
                    proxyId={form.watch("proxy_id")}
                    proxies={proxies}
                    invalid={Boolean(errors.proxy)}
                    onChange={(next) => {
                      field.onChange(next.proxy)
                      form.setValue("proxy_id", next.proxy_id)
                    }}
                  />
                )}
              />
              <FieldDescription>A saved proxy follows later edits. A typed URL stays on this account.</FieldDescription>
              <FieldError errors={[errors.proxy]} />
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
                      if (next === "active" || next === "inactive") field.onChange(next)
                    }}
                  >
                    <ToggleGroupItem value="active" data-tone="positive">Active</ToggleGroupItem>
                    <ToggleGroupItem value="inactive" data-tone="negative">Inactive</ToggleGroupItem>
                  </ToggleGroup>
                )}
              />
            </Field>
            <Field data-invalid={Boolean(errors.description) || undefined}>
              <FieldLabel htmlFor="description">Description</FieldLabel>
              <Textarea id="description" rows={3} aria-invalid={Boolean(errors.description) || undefined} {...form.register("description")} />
              <FieldDescription>Optional note, up to 280 characters.</FieldDescription>
              <FieldError errors={[errors.description]} />
            </Field>
            <Field orientation="horizontal">
              <Controller
                control={form.control}
                name="show_limit"
                render={({ field }) => <Switch id="show_limit" checked={field.value} onCheckedChange={field.onChange} />}
              />
              <FieldLabel htmlFor="show_limit">Show limit</FieldLabel>
            </Field>
          </FieldGroup>
        </div>
        {role === "master" ? (
        <div className={tab === "advanced" ? "flex flex-col gap-4" : "hidden"}>
          <FieldGroup>
              <>
                <Field>
                  <FieldLabel htmlFor="automation_task_uid">GoAuto task UID</FieldLabel>
                  <Input id="automation_task_uid" placeholder="gfx_runSeositecheckup" {...form.register("automation_task_uid")} />
                </Field>
                <Field data-invalid={Boolean(errors.automation_ingest_key) || undefined}>
                  <FieldLabel htmlFor="automation_ingest_key">Automation ingest key</FieldLabel>
                  <Input
                    id="automation_ingest_key"
                    className="font-mono"
                    aria-invalid={Boolean(errors.automation_ingest_key) || undefined}
                    {...form.register("automation_ingest_key")}
                  />
                  <FieldDescription>Optional. Lowercase letters, numbers, and underscores. Unique across accounts.</FieldDescription>
                  <FieldError errors={[errors.automation_ingest_key]} />
                </Field>
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <span>Last ingest</span>
                  {latestIngest ? <StatusBadge status={latestIngest.status} /> : <span>—</span>}
                  <span className="whitespace-nowrap">{latestIngest ? formatTime(latestIngest.created_at) : ""}</span>
                </div>
              </>
          </FieldGroup>
        </div>
        ) : null}
        <div className="flex w-full flex-wrap items-center justify-between gap-3">
        {account ? (
          <Button type="button" variant="destructive" onClick={() => setConfirmDelete(true)}>
            Delete
          </Button>
        ) : <span />}
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save account
        </Button>
        </div>
      </form>
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title="Delete account"
        description={account ? `Delete ${account.name}? Sessions using it return to auto-switch.` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!account) return
          setDeleting(true)
          void runMutation(() => deleteAccount(account.id), "Account deleted").then((ok) => {
            setDeleting(false)
            if (ok) onDone()
          })
        }}
      />
    </>
  )
}

function ProxyField({
  id,
  value,
  proxyId,
  proxies,
  invalid,
  onChange,
}: {
  id: string
  value: string
  proxyId: string
  proxies: ProxyEndpoint[]
  invalid?: boolean
  onChange: (value: { proxy: string; proxy_id: string }) => void
}) {
  const manual = !proxyId && Boolean(value)
  const selected = manual ? "manual" : proxyId || "none"
  return (
    <div className="flex flex-col gap-2">
      <ChoiceSelect
        id={id}
        value={selected}
        invalid={invalid}
        items={[
          { label: "No proxy", value: "none" },
          ...proxies.map((proxy) => ({ label: `${proxy.name} (${proxy.proxy_type})`, value: String(proxy.id) })),
          { label: "Manual URL", value: "manual" },
        ]}
        onValueChange={(next) => {
          if (next === "manual") {
            onChange({ proxy: value.startsWith("socks5://") || value.startsWith("http") ? value : "", proxy_id: "" })
            return
          }
          if (next === "none") {
            onChange({ proxy: "", proxy_id: "" })
            return
          }
          const proxy = proxies.find((item) => String(item.id) === next)
          onChange({ proxy: proxy?.endpoint ?? "", proxy_id: next })
        }}
      />
      {selected === "manual" ? (
        <Input
          aria-invalid={invalid || undefined}
          value={value}
          placeholder="socks5://10.0.0.1:1080"
          onChange={(event) => onChange({ proxy: event.target.value, proxy_id: "" })}
        />
      ) : null}
    </div>
  )
}
