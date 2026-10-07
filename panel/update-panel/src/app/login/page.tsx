"use client"

import { useEffect, useRef } from "react"
import { useRouter } from "next/navigation"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { toast } from "sonner"

import { useAuth } from "@/components/auth-provider"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { gsap, prefersReducedMotion, useGSAP } from "@/lib/gsap"

const schema = z.object({
  username: z.string().trim().min(1, "Enter a username"),
  password: z.string().min(8, "Use at least 8 characters"),
})

type FormValues = z.infer<typeof schema>

export default function LoginPage() {
  const router = useRouter()
  const { session, ready, signIn } = useAuth()
  const scope = useRef<HTMLDivElement>(null)
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { username: "", password: "" },
  })

  useEffect(() => {
    if (ready && session) router.replace("/")
  }, [ready, session, router])

  useGSAP(
    () => {
      const card = scope.current?.querySelector("[data-login-card]")
      if (!(card instanceof HTMLElement)) return
      // Always end visible — a failed/interrupted from(autoAlpha:0) left the page blank.
      if (prefersReducedMotion()) {
        gsap.set(card, { autoAlpha: 1, y: 0 })
        return
      }
      gsap.fromTo(
        card,
        { autoAlpha: 0, y: 16 },
        { autoAlpha: 1, y: 0, duration: 0.45, ease: "power2.out", overwrite: true }
      )
    },
    { scope }
  )

  async function onSubmit(values: FormValues) {
    try {
      await signIn(values.username, values.password)
      router.replace("/")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Sign in failed")
    }
  }

  return (
    <main ref={scope} className="flex min-h-svh items-center justify-center bg-background p-4">
      <Card data-login-card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>ToolsMandi</CardTitle>
          <CardDescription>
            Sign in with master / toolsmandi.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={form.handleSubmit(onSubmit)}>
            <FieldGroup>
              <Field data-invalid={Boolean(form.formState.errors.username) || undefined}>
                <FieldLabel htmlFor="username">Username</FieldLabel>
                <Input
                  id="username"
                  autoComplete="username"
                  aria-invalid={Boolean(form.formState.errors.username) || undefined}
                  {...form.register("username")}
                />
                <FieldError errors={[form.formState.errors.username]} />
              </Field>
              <Field data-invalid={Boolean(form.formState.errors.password) || undefined}>
                <FieldLabel htmlFor="password">Password</FieldLabel>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  aria-invalid={Boolean(form.formState.errors.password) || undefined}
                  {...form.register("password")}
                />
                <FieldError errors={[form.formState.errors.password]} />
              </Field>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting ? <Spinner data-icon="inline-start" /> : null}
                Sign in
              </Button>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
    </main>
  )
}
