"use client"

import { useState } from "react"
import { Controller, useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"

import { useAuth } from "@/components/auth-provider"
import { ChoiceSelect } from "@/components/choice-select"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import { listAllWebsites, openToolAccess } from "@/lib/api/client"

const schema = z.object({
  website_id: z.string().min(1, "Choose a tool"),
  username: z.string().trim().min(1, "Enter a username"),
  product_id: z.string().trim().min(1, "Enter a product id"),
})

type FormValues = z.infer<typeof schema>

async function browserPublicIP(): Promise<string> {
  const checks = [
    async () => (await (await fetch("https://ipv4.icanhazip.com", { cache: "no-store" })).text()).trim(),
    async () => (await (await fetch("https://ifconfig.me/ip", { cache: "no-store" })).text()).trim(),
    async () => {
      const body = await (await fetch("https://api.ipify.org?format=json", { cache: "no-store" })).json() as { ip?: string }
      return (body.ip ?? "").trim()
    },
  ]
  for (const check of checks) {
    try {
      const ip = await check()
      if (/^(?:\d{1,3}\.){3}\d{1,3}$/.test(ip) && !ip.startsWith("127.") && !ip.startsWith("10.") && !ip.startsWith("192.168.") && !/^172\.(1[6-9]|2\d|3[01])\./.test(ip)) {
        return ip
      }
    } catch {
      continue
    }
  }
  return ""
}

export default function AccessPage() {
  const { session } = useAuth()
  const { data, loading } = useResource(session?.role ?? "none", listAllWebsites)
  const [result, setResult] = useState<{ allowed: boolean; error?: string; tool?: string; username?: string; product_id?: string } | null>(null)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { website_id: "", username: "", product_id: "" },
  })

  async function onSubmit(values: FormValues) {
    setResult(null)
    const clientIP = await browserPublicIP()
    if (!clientIP) {
      setResult({ allowed: false, error: "This browser's IP could not be read. Check the connection and try again." })
      return
    }
    let opened
    try {
      opened = await openToolAccess({
        website_id: Number(values.website_id),
        username: values.username,
        product_id: values.product_id,
        client_ip: clientIP,
      })
    } catch (err) {
      setResult({ allowed: false, error: err instanceof Error ? err.message : "Access could not be opened." })
      return
    }
    if (!opened.allowed || !opened.open_url) {
      setResult({ allowed: false, error: opened.error || "Access denied" })
      return
    }
    const popup = window.open(opened.open_url, "_blank")
    if (!popup) {
      setResult({ allowed: false, error: "The browser blocked the new tab. Allow pop-ups for this site and try again." })
      return
    }
    popup.opener = null
  }

  const websites = data ?? []

  return (
    <>
      <PageHeader
        title="Access"
        description="This button stands in for the aMember access button. It checks the username and product id, then returns you to the tool."
      />
      <Card className="max-w-lg">
        <CardContent>
          <form onSubmit={form.handleSubmit(onSubmit)}>
            <FieldGroup>
              <Field data-invalid={Boolean(form.formState.errors.website_id) || undefined}>
                <FieldLabel htmlFor="website_id">Tool</FieldLabel>
                <Controller
                  name="website_id"
                  control={form.control}
                  render={({ field }) => (
                    <ChoiceSelect
                      id="website_id"
                      value={field.value}
                      onValueChange={field.onChange}
                      invalid={Boolean(form.formState.errors.website_id)}
                      placeholder={loading ? "Loading tools" : "Choose a tool"}
                      items={websites.map((website) => ({
                        label: `${website.name} — ${website.domain}`,
                        value: String(website.id),
                      }))}
                    />
                  )}
                />
                <FieldError errors={[form.formState.errors.website_id]} />
              </Field>
              <Field data-invalid={Boolean(form.formState.errors.username) || undefined}>
                <FieldLabel htmlFor="username">Username</FieldLabel>
                <Input id="username" autoComplete="off" aria-invalid={Boolean(form.formState.errors.username) || undefined} {...form.register("username")} />
                <FieldError errors={[form.formState.errors.username]} />
              </Field>
              <Field data-invalid={Boolean(form.formState.errors.product_id) || undefined}>
                <FieldLabel htmlFor="product_id">Product id</FieldLabel>
                <Input id="product_id" inputMode="numeric" aria-invalid={Boolean(form.formState.errors.product_id) || undefined} {...form.register("product_id")} />
                <FieldError errors={[form.formState.errors.product_id]} />
              </Field>
              <Button type="submit" disabled={form.formState.isSubmitting || loading}>
                {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
                Open
              </Button>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      {result && !result.allowed ? (
        <Alert variant="destructive" className="max-w-lg">
          <AlertTitle>Access denied</AlertTitle>
          <AlertDescription>{result.error}</AlertDescription>
        </Alert>
      ) : null}
    </>
  )
}
