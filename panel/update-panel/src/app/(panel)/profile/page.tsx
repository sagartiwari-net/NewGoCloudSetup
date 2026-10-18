"use client"

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
import { changePassword } from "@/lib/api/client"
import { runMutation } from "@/lib/mutate"

const passwordSchema = z.object({
  password: z.string().min(8, "Use at least 8 characters"),
})

type PasswordValues = z.infer<typeof passwordSchema>

export default function ProfilePage() {
  const { session } = useAuth()
  return (
    <>
      <PageHeader title="Profile" description={`Signed in as ${session?.username ?? "operator"}.`} />
      <Card className="max-w-lg">
        <CardContent className="text-sm text-muted-foreground">
          Role: {session?.role ?? "unknown"}
        </CardContent>
      </Card>
      <PasswordCard />
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
        <CardDescription>Updates the password for this signed-in account.</CardDescription>
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
