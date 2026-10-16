"use client"

import { useMemo, useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { PlusIcon } from "lucide-react"

import { ChoiceSelect } from "@/components/choice-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DataTable, type DataColumn } from "@/components/data-table"
import { FormOverlay } from "@/components/form-overlay"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { useResource } from "@/hooks/use-resource"
import {
  automationOptions,
  deleteAutomationJob,
  listAutomationJobs,
  runAutomationJob,
  saveAutomationJob,
} from "@/lib/api/client"
import type { AutomationJob, AutomationOptions, AutomationWebsite } from "@/lib/api/types"
import { formatIst } from "@/lib/format"
import { runMutation } from "@/lib/mutate"
import { useListPage } from "@/lib/page-size"

function websiteLabel(website: Pick<AutomationWebsite, "name" | "domain">) {
  const domain = website.domain.trim()
  if (!domain || domain.toLowerCase() === website.name.trim().toLowerCase()) return website.name
  return `${website.name} · ${domain}`
}

function jobSchema(websites: AutomationWebsite[]) {
  return z
    .object({
      name: z.string().trim().min(1, "Enter a name"),
      provider: z.string().min(1, "Choose a provider"),
      credential_id: z.string().min(1, "Choose a credential"),
      target_website_id: z.string(),
      target_account_id: z.string(),
      job_kind: z.enum(["tool", "session"]),
      interval_minutes: z
        .number()
        .int()
        .min(0, "Use 0 to turn the timer off, or 5 to 1440 minutes")
        .max(1440, "Use 0 to turn the timer off, or 5 to 1440 minutes")
        .refine((value) => value === 0 || value >= 5, "Use 0 to turn the timer off, or 5 to 1440 minutes"),
      enabled: z.boolean(),
      wake_website_ids: z.array(z.string()),
    })
    .superRefine((value, ctx) => {
      if (value.job_kind !== "session" && value.target_website_id === "") {
        ctx.addIssue({ code: "custom", path: ["target_website_id"], message: "Choose a website" })
      }
      const website = websites.find((item) => String(item.id) === value.target_website_id)
      if (value.job_kind !== "session" && website && website.kind !== "recloud" && value.target_account_id === "") {
        ctx.addIssue({ code: "custom", path: ["target_account_id"], message: "Choose an account" })
      }
    })
}

type FormValues = z.infer<ReturnType<typeof jobSchema>>

export default function AutomationJobsPage() {
  const [query, setQuery] = useState("")
  const { page, setPage, pageSize } = useListPage()
  const { data, loading, reload } = useResource(`automation-jobs-${page}-${pageSize}-${query}`, async () => {
    const [jobs, options] = await Promise.all([
      listAutomationJobs({ page, pageSize, query }),
      automationOptions(),
    ])
    return { jobs, options }
  })
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<AutomationJob | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AutomationJob | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [running, setRunning] = useState<number | null>(null)
  const rows = data?.jobs.items ?? []
  const options = data?.options

  const columns: DataColumn<AutomationJob>[] = [
    { id: "name", header: "Name", cell: (row) => row.name, sortValue: (row) => row.name },
    {
      id: "credential",
      header: "Credential",
      cell: (row) => row.credential_name || "—",
      sortValue: (row) => row.credential_name,
    },
    { id: "target", header: "Target", cell: (row) => row.target_label || "—", sortValue: (row) => row.target_label },
    {
      id: "interval",
      header: "Interval",
      cell: (row) => (row.interval_minutes === 0 ? "Off" : `${row.interval_minutes} min`),
      sortValue: (row) => row.interval_minutes,
    },
    {
      id: "enabled",
      header: "Enabled",
      cell: (row) => (row.enabled ? "Yes" : "No"),
      sortValue: (row) => (row.enabled ? 1 : 0),
    },
    {
      id: "last",
      header: "Last run",
      cell: (row) => (
        <div className="flex min-w-0 flex-col gap-1">
          <span>{row.last_run_at ? formatIst(row.last_run_at) : "—"}</span>
          {row.last_result ? (
            <Badge variant={row.last_result === "ok" ? "fresh" : "destructive"}>{row.last_result}</Badge>
          ) : null}
          {row.last_reason ? <span className="text-sm text-muted-foreground">{row.last_reason}</span> : null}
        </div>
      ),
      sortValue: (row) => row.last_run_at,
    },
    {
      id: "actions",
      header: "Actions",
      cell: (row) => (
        <div className="flex flex-wrap gap-3">
          <Button
            variant="outline"
            size="sm"
            disabled={running === row.id}
            onClick={() => {
              setRunning(row.id)
              void runMutation(() => runAutomationJob(row.id), "Run started").then((ok) => {
                setRunning(null)
                if (ok) reload()
              })
            }}
          >
            {running === row.id ? <Spinner data-icon="inline-start" /> : null}
            Run now
          </Button>
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
        title="Jobs"
        description="Each job checks one saved session and writes the tool cookie onto the website you pick."
        action={
          <Button
            onClick={() => {
              setEditing(null)
              setOpen(true)
            }}
          >
            <PlusIcon data-icon="inline-start" />
            New job
          </Button>
        }
      />
      <Field className="md:max-w-sm">
        <FieldLabel htmlFor="job-search">Search</FieldLabel>
        <Input
          id="job-search"
          value={query}
          placeholder="Job, credential, or website"
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
              page: data?.jobs.page ?? page,
              total: data?.jobs.total ?? 0,
              pageSize,
              onPageChange: setPage,
            }}
            resetKey={query}
            emptyTitle={query ? "No matches" : "No jobs"}
            emptyDescription="Save a job with a website, an interval, and Run now."
          />
        </CardContent>
      </Card>
      <FormOverlay
        open={open}
        onOpenChange={setOpen}
        title={editing ? "Edit job" : "New job"}
        description="Pick the website and the account that should receive the cookie. Own tools do not need a recloud upstream URL."
      >
        {open && options ? (
          <JobForm
            key={editing?.id ?? "new"}
            job={editing}
            options={options}
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
        title="Delete job"
        description={pendingDelete ? `Delete ${pendingDelete.name}?` : ""}
        confirmLabel="Delete"
        pending={deleting}
        onConfirm={() => {
          if (!pendingDelete) return
          setDeleting(true)
          void runMutation(() => deleteAutomationJob(pendingDelete.id), "Job deleted").then((ok) => {
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

function JobForm({
  job,
  options,
  onDone,
}: {
  job: AutomationJob | null
  options: AutomationOptions
  onDone: () => void
}) {
  const schema = useMemo(() => jobSchema(options.websites), [options.websites])
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: job?.name ?? "",
      provider: job?.provider ?? options.providers[0]?.id ?? "",
      credential_id: job ? String(job.credential_id) : "",
      target_website_id: job ? String(job.target_website_id) : "",
      target_account_id: job && job.target_account_id > 0 ? String(job.target_account_id) : "",
      interval_minutes: job?.interval_minutes ?? 60,
      enabled: job?.enabled ?? true,
      job_kind: job?.job_kind === "session" ? "session" : "tool",
      wake_website_ids: (job?.wake_website_ids ?? []).map(String),
    },
  })
  const errors = form.formState.errors
  const provider = form.watch("provider")
  const websiteId = form.watch("target_website_id")
  const website = options.websites.find((item) => String(item.id) === websiteId)
  const credentials = options.credentials.filter((item) => item.provider === provider)
  const jobKind = form.watch("job_kind")
  const showAccount = jobKind !== "session" && Boolean(website) && website?.kind !== "recloud"
  const toolOnly = provider === "ahrefs" || provider === "zikaanalytics" || provider === "erank"
  const wakeSites = options.websites.filter((item) => {
    if (String(item.id) === websiteId) return false
    if (provider === "ahrefs") return Boolean(website) && item.tool_id === website.tool_id
    return item.kind === "recloud"
  })
  const selectedWakes = form.watch("wake_website_ids")

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={form.handleSubmit(async (values) => {
        const selected = options.websites.find((item) => String(item.id) === values.target_website_id)
        const ok = await runMutation(
          () =>
            saveAutomationJob({
              id: job?.id,
              name: values.name.trim(),
              provider: values.provider,
              credential_id: Number(values.credential_id),
              target_website_id: values.job_kind === "session" ? 0 : Number(values.target_website_id),
              target_account_id: values.job_kind === "session" || selected?.kind === "recloud" ? 0 : Number(values.target_account_id),
              interval_minutes: values.interval_minutes,
              enabled: values.enabled,
              job_kind: values.job_kind,
              wake_website_ids: values.job_kind === "session" ? [] : values.wake_website_ids.map(Number).filter((id) => id > 0 && id !== Number(values.target_website_id)),
            }),
          "Job saved",
        )
        if (ok) onDone()
      })}
    >
      <FieldGroup>
        <Field data-invalid={Boolean(errors.name) || undefined}>
          <FieldLabel htmlFor="job-name">Name</FieldLabel>
          <Input id="job-name" placeholder="Markhor Semrush 1" {...form.register("name")} />
          <FieldError errors={[errors.name]} />
        </Field>
        <Field data-invalid={Boolean(errors.provider) || undefined}>
          <FieldLabel htmlFor="job-provider">Provider</FieldLabel>
          <ChoiceSelect
            id="job-provider"
            value={provider}
            onValueChange={(value) => {
              form.setValue("provider", value, { shouldValidate: true })
              if (value === "ahrefs" || value === "zikaanalytics" || value === "erank") {
                form.setValue("job_kind", "tool", { shouldValidate: true })
              }
              const still = options.credentials.find(
                (item) => String(item.id) === form.getValues("credential_id") && item.provider === value,
              )
              if (!still) form.setValue("credential_id", "")
            }}
            items={options.providers.map((item) => ({ label: item.label, value: item.id }))}
            placeholder="Choose a provider"
            invalid={Boolean(errors.provider)}
          />
          <FieldError errors={[errors.provider]} />
        </Field>
        <Field data-invalid={Boolean(errors.credential_id) || undefined}>
          <FieldLabel htmlFor="job-credential">Credential</FieldLabel>
          <ChoiceSelect
            id="job-credential"
            value={form.watch("credential_id")}
            onValueChange={(value) => form.setValue("credential_id", value, { shouldValidate: true })}
            items={credentials.map((item) => ({ label: `${item.name} · ${item.username}`, value: String(item.id) }))}
            placeholder="Choose a credential"
            invalid={Boolean(errors.credential_id)}
          />
          <FieldError errors={[errors.credential_id]} />
          {provider === "ahrefs" && credentials.length === 0 ? (
            <FieldDescription>
              Create an Ahrefs credential first. Leave the password blank, then paste the cookie and its user agent on Storage.
            </FieldDescription>
          ) : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="job-kind">Job</FieldLabel>
          <ChoiceSelect
            id="job-kind"
            value={jobKind}
            onValueChange={(value) => form.setValue("job_kind", !toolOnly && value === "session" ? "session" : "tool", { shouldValidate: true })}
            items={
              toolOnly
                ? [{ label: "Update one tool", value: "tool" }]
                : [
                    { label: "Update one tool", value: "tool" },
                    { label: "Store the homepage cookie", value: "session" },
                  ]
            }
          />
          <FieldDescription>
            {provider === "ahrefs"
              ? "Refreshes the cookie saved in Storage and writes it to the target and the Ahrefs sites you tick below."
              : "Homepage only keeps the portal login. A tool job uses that cookie and writes one tool."}
          </FieldDescription>
        </Field>
        {jobKind === "session" ? null : (
        <Field data-invalid={Boolean(errors.target_website_id) || undefined}>
          <FieldLabel htmlFor="job-website">Target website</FieldLabel>
          <ChoiceSelect
            id="job-website"
            value={websiteId}
            onValueChange={(value) => {
              form.setValue("target_website_id", value, { shouldValidate: true })
              const next = options.websites.find((item) => String(item.id) === value)
              if (!next || next.kind === "recloud") form.setValue("target_account_id", "")
            }}
            items={options.websites.map((item) => ({ label: websiteLabel(item), value: String(item.id) }))}
            placeholder="Choose a website"
            invalid={Boolean(errors.target_website_id)}
          />
          <FieldError errors={[errors.target_website_id]} />
        </Field>
        )}
        {showAccount ? (
          <Field data-invalid={Boolean(errors.target_account_id) || undefined}>
            <FieldLabel htmlFor="job-account">Account</FieldLabel>
            <ChoiceSelect
              id="job-account"
              value={form.watch("target_account_id")}
              onValueChange={(value) => form.setValue("target_account_id", value, { shouldValidate: true })}
              items={(website?.accounts ?? []).map((item) => ({ label: item.name, value: String(item.id) }))}
              placeholder="Choose an account"
              invalid={Boolean(errors.target_account_id)}
            />
            <FieldError errors={[errors.target_account_id]} />
          </Field>
        ) : null}
        <Field data-invalid={Boolean(errors.interval_minutes) || undefined}>
          <FieldLabel htmlFor="job-interval">Interval (minutes)</FieldLabel>
          <Input id="job-interval" type="number" min={0} max={1440} {...form.register("interval_minutes", { valueAsNumber: true })} />
          <FieldDescription>
            0 turns the timer off. Logout and Run now still run while Enabled is on. Turn Enabled off to stop the timer, logout, and Run now.
          </FieldDescription>
          <FieldError errors={[errors.interval_minutes]} />
        </Field>
        {jobKind !== "session" && wakeSites.length > 0 ? (
          <Field>
            <FieldLabel>Also run when these log out</FieldLabel>
            <FieldDescription>
              {provider === "ahrefs"
                ? "Tick the other websites of this same Ahrefs tool. Each run writes this cookie onto their first account."
                : "Leave this empty while this is the only job for the provider. Every matching recloud tool will wake it. Choose sites when one provider has more than one job."}
            </FieldDescription>
            <div className="flex max-h-48 flex-col gap-2 overflow-auto rounded-md border p-3">
              {wakeSites.map((item) => {
                const id = String(item.id)
                const checked = selectedWakes.includes(id)
                return (
                  <label key={item.id} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={checked}
                      onCheckedChange={(value) => {
                        const next = new Set(selectedWakes)
                        if (value === true) next.add(id)
                        else next.delete(id)
                        form.setValue("wake_website_ids", [...next], { shouldDirty: true })
                      }}
                    />
                    <span>{websiteLabel(item)}</span>
                  </label>
                )
              })}
            </div>
          </Field>
        ) : null}
        <Field orientation="horizontal">
          <Controller
            control={form.control}
            name="enabled"
            render={({ field }) => <Switch id="job-enabled" checked={field.value} onCheckedChange={field.onChange} />}
          />
          <FieldLabel htmlFor="job-enabled">Enabled</FieldLabel>
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
