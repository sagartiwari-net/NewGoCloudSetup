"use client"

import { useMemo, useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

import { ChoiceSelect } from "@/components/choice-select"
import { useAuth } from "@/components/auth-provider"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { useResource } from "@/hooks/use-resource"
import {
  getAhrefsBarSettings,
  listAhrefsBarCosts,
  listResellerChoices,
  saveAhrefsBarCost,
  saveAhrefsBarSettings,
} from "@/lib/api/client"
import { runMutation } from "@/lib/mutate"

const schema = z.object({
  hourly_limit: z.number().int().min(0),
  daily_limit: z.number().int().min(0),
  cache_ttl_hours: z.number().int().min(1),
  sticky_minutes: z.number().int().min(1),
  refund_on_5xx: z.enum(["yes", "no"]),
})

type FormValues = z.infer<typeof schema>

function costClassHint(cls: string) {
  if (cls === "billable") return "Counts against hourly/daily credit limits"
  if (cls === "cheap") return "Upstream call, 0 credits (does not burn user limit)"
  if (cls === "free") return "Local/synthetic response — 0 credits, no Ahrefs meter burn"
  return cls
}

export default function AhrefsBarDefaultsPage() {
  const { session } = useAuth()
  const isMaster = session?.role === "master"
  const [resellerId, setResellerId] = useState(0)

  const { data: resellers } = useResource(isMaster ? "ab-reseller-choices" : "skip", async () => {
    if (!isMaster) return { items: [] as Array<{ id: number; username: string; ahrefs_bar_enabled?: boolean }> }
    const page = await listResellerChoices()
    return { items: page.items.filter((r) => r.ahrefs_bar_enabled) }
  })

  const effectiveResellerId = isMaster ? resellerId : 0
  const { data, loading, reload } = useResource(
    `${session?.role ?? "none"}-defaults-${effectiveResellerId}`,
    async () => {
      const [settings, costs] = await Promise.all([
        getAhrefsBarSettings(isMaster && resellerId > 0 ? resellerId : undefined),
        listAhrefsBarCosts(),
      ])
      return { settings, costs: costs.items }
    },
  )

  const resellerItems = useMemo(() => {
    const rows = resellers?.items ?? []
    return [
      { label: "Master (own defaults)", value: "0" },
      ...rows.map((r) => ({ label: r.username, value: String(r.id) })),
    ]
  }, [resellers])

  return (
    <>
      <PageHeader
        title="Ahrefs Bar limits"
        description={
          isMaster
            ? "Set hourly/daily credit caps per reseller (pick one with Bar access). Endpoint cost classes are global."
            : "Your reseller credit caps for toolbar users. Endpoint costs are set by master."
        }
      />
      {isMaster ? (
        <Field className="max-w-md">
          <FieldLabel htmlFor="ab-reseller">Reseller</FieldLabel>
          <ChoiceSelect
            id="ab-reseller"
            value={String(resellerId)}
            onValueChange={(v) => setResellerId(Number(v) || 0)}
            items={resellerItems}
          />
          <FieldDescription>
            Only resellers with Ahrefs Bar access appear here. Enable access under Catalog → Resellers.
          </FieldDescription>
        </Field>
      ) : null}
      {loading && !data ? (
        <div className="flex justify-center py-16"><Spinner /></div>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Credit & cache</CardTitle>
            </CardHeader>
            <CardContent>
              {data ? (
                <SettingsForm
                  key={`${data.settings.reseller_id}-${data.settings.updated_at}-${effectiveResellerId}`}
                  initial={data.settings}
                  resellerId={isMaster ? resellerId || data.settings.reseller_id : undefined}
                  onDone={reload}
                />
              ) : null}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Endpoint costs</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-muted-foreground">
                <span className="font-medium">free</span> / <span className="font-medium">cheap</span> = 0 credits.
                Only <span className="font-medium">billable</span> (e.g. keywords) burns the hourly/daily limit.
              </p>
              {(data?.costs ?? []).map((row) => (
                <div key={row.endpoint} className="flex flex-wrap items-center gap-3 rounded-lg border p-3">
                  <div className="min-w-0 flex-1">
                    <div className="font-mono text-sm">{row.endpoint}</div>
                    <div className="text-xs text-muted-foreground">
                      {row.class} — {costClassHint(row.class)}
                    </div>
                  </div>
                  {isMaster ? (
                    <CostEditor
                      endpoint={row.endpoint}
                      credits={row.credits}
                      costClass={row.class}
                      onDone={reload}
                    />
                  ) : (
                    <div className="tabular-nums text-sm">{row.credits} credit(s)</div>
                  )}
                </div>
              ))}
              {!data?.costs?.length ? (
                <p className="text-sm text-muted-foreground">No cost rows yet — API will seed on start.</p>
              ) : null}
            </CardContent>
          </Card>
        </div>
      )}
    </>
  )
}

function SettingsForm({
  initial,
  resellerId,
  onDone,
}: {
  initial: {
    hourly_limit: number
    daily_limit: number
    cache_ttl_hours: number
    sticky_minutes: number
    refund_on_5xx: boolean
  }
  resellerId?: number
  onDone: () => void
}) {
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      hourly_limit: initial.hourly_limit,
      daily_limit: initial.daily_limit,
      cache_ttl_hours: initial.cache_ttl_hours,
      sticky_minutes: initial.sticky_minutes,
      refund_on_5xx: initial.refund_on_5xx ? "yes" : "no",
    },
  })
  const errors = form.formState.errors

  async function onSubmit(values: FormValues) {
    const ok = await runMutation(
      () =>
        saveAhrefsBarSettings({
          reseller_id: resellerId && resellerId > 0 ? resellerId : undefined,
          hourly_limit: values.hourly_limit,
          daily_limit: values.daily_limit,
          cache_ttl_hours: values.cache_ttl_hours,
          sticky_minutes: values.sticky_minutes,
          refund_on_5xx: values.refund_on_5xx === "yes",
        }),
      "Limits saved",
    )
    if (ok) onDone()
  }

  return (
    <form onSubmit={form.handleSubmit(onSubmit)}>
      <FieldGroup>
        <Field data-invalid={Boolean(errors.hourly_limit) || undefined}>
          <FieldLabel htmlFor="ab-hourly">Hourly limit</FieldLabel>
          <Input id="ab-hourly" type="number" {...form.register("hourly_limit", { valueAsNumber: true })} />
          <FieldDescription>Max billable credits per rolling hour for this reseller&apos;s bar users.</FieldDescription>
          <FieldError errors={[errors.hourly_limit]} />
        </Field>
        <Field data-invalid={Boolean(errors.daily_limit) || undefined}>
          <FieldLabel htmlFor="ab-daily">Daily limit (IST)</FieldLabel>
          <Input id="ab-daily" type="number" {...form.register("daily_limit", { valueAsNumber: true })} />
          <FieldError errors={[errors.daily_limit]} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-ttl">Browser cache TTL (hours)</FieldLabel>
          <Input id="ab-ttl" type="number" {...form.register("cache_ttl_hours", { valueAsNumber: true })} />
          <FieldDescription>Default 72. Extension IndexedDB uses this later.</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="ab-sticky">Sticky minutes</FieldLabel>
          <Input id="ab-sticky" type="number" {...form.register("sticky_minutes", { valueAsNumber: true })} />
          <FieldDescription>
            After a successful Ahrefs call, keep the same pool token for this many minutes (fewer account switches / 429s).
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel>Refund on upstream 5xx</FieldLabel>
          <Controller
            control={form.control}
            name="refund_on_5xx"
            render={({ field }) => (
              <ToggleGroup
                spacing={2}
                value={[field.value]}
                onValueChange={(values) => {
                  const next = values[values.length - 1]
                  if (next === "yes" || next === "no") field.onChange(next)
                }}
              >
                <ToggleGroupItem value="yes" data-tone="positive">Yes</ToggleGroupItem>
                <ToggleGroupItem value="no">No</ToggleGroupItem>
              </ToggleGroup>
            )}
          />
        </Field>
        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
          Save limits
        </Button>
      </FieldGroup>
    </form>
  )
}

function CostEditor({
  endpoint,
  credits,
  costClass,
  onDone,
}: {
  endpoint: string
  credits: number
  costClass: string
  onDone: () => void
}) {
  const form = useForm({
    defaultValues: { credits },
  })

  return (
    <form
      className="flex items-center gap-2"
      onSubmit={form.handleSubmit(async (values) => {
        const ok = await runMutation(
          () => saveAhrefsBarCost({ endpoint, credits: Number(values.credits), class: costClass }),
          "Cost updated",
        )
        if (ok) onDone()
      })}
    >
      <Input className="w-20" type="number" {...form.register("credits")} />
      <Button type="submit" size="sm" variant="outline" disabled={form.formState.isSubmitting}>
        Save
      </Button>
    </form>
  )
}
