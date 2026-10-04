"use client"

import { useState } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

import { useAuth } from "@/components/auth-provider"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { changePassword, getAccessAlerts, saveAccessAlerts } from "@/lib/api/client"
import type { AccessAlerts } from "@/lib/api/types"
import { runMutation } from "@/lib/mutate"

const passwordSchema = z.object({
  password: z.string().min(8, "Use at least 8 characters"),
})

type PasswordValues = z.infer<typeof passwordSchema>

export default function SettingsPage() {
  const { session } = useAuth()
  if (session?.role !== "master") return null
  return (
    <>
      <PageHeader title="Settings" description="Master settings. Resellers cannot open or change this page." />
      <PasswordCard />
      <AccessAlertSettings />
    </>
  )
}

function PasswordCard() {
  const form = useForm<PasswordValues>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { password: "" },
  })

  async function onSubmit(values: PasswordValues) {
    const ok = await runMutation(() => changePassword(values.password), "Password updated")
    if (ok) form.reset()
  }

  return (
    <Card className="max-w-lg">
      <CardHeader>
        <CardTitle>Change password</CardTitle>
        <CardDescription>Updates the password for the signed-in master account.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={form.handleSubmit(onSubmit)}>
          <FieldGroup>
            <Field data-invalid={Boolean(form.formState.errors.password) || undefined}>
              <FieldLabel htmlFor="password">New password</FieldLabel>
              <Input
                id="password"
                type="password"
                autoComplete="new-password"
                aria-invalid={Boolean(form.formState.errors.password) || undefined}
                {...form.register("password")}
              />
              <FieldError errors={[form.formState.errors.password]} />
            </Field>
            <Button type="submit" disabled={form.formState.isSubmitting}>
              {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
              Update password
            </Button>
          </FieldGroup>
        </form>
      </CardContent>
    </Card>
  )
}

function AccessAlertSettings() {
  const { data, reload } = useResource("access-alerts", getAccessAlerts)
  const [form, setForm] = useState<AccessAlerts | null>(null)
  const [saving, setSaving] = useState(false)
  const values = form ?? data
  if (!values) return null
  function setNumber(key: keyof AccessAlerts, raw: string) {
    const next = Number(raw)
    setForm({ ...(form ?? data)!, [key]: Number.isFinite(next) ? next : 0 })
  }
  const fields: Array<{ key: keyof AccessAlerts; label: string }> = [
    { key: "same_tool_count", label: "Same tool, how many opens" },
    { key: "same_tool_minutes", label: "Same tool, within minutes" },
    { key: "multi_tool_count", label: "Different tools, how many" },
    { key: "multi_tool_minutes", label: "Different tools, within minutes" },
    { key: "repeat_minutes", label: "Repeat gap, minutes or less" },
    { key: "repeat_hours", label: "Repeat must last, hours" },
    { key: "repeat_min_opens", label: "Repeat, minimum opens" },
  ]
  return (
    <Card>
      <CardHeader>
        <CardTitle>Access alerts</CardTitle>
        <CardDescription>A Security row is written when any of these is true for one username.</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid gap-3 md:grid-cols-2">
          {fields.map((field) => (
            <Field key={field.key}>
              <FieldLabel htmlFor={`alert-${field.key}`}>{field.label}</FieldLabel>
              <Input
                id={`alert-${field.key}`}
                type="number"
                min={1}
                value={values[field.key]}
                onChange={(event) => setNumber(field.key, event.target.value)}
              />
            </Field>
          ))}
        </div>
        <Button
          className="w-fit"
          disabled={saving}
          onClick={() => {
            setSaving(true)
            void runMutation(() => saveAccessAlerts(values), "Access alerts saved").then((ok) => {
              setSaving(false)
              if (ok) {
                setForm(null)
                void reload()
              }
            })
          }}
        >
          Save
        </Button>
      </CardContent>
    </Card>
  )
}
