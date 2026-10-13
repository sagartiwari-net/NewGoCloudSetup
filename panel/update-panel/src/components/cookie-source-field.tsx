"use client"

import { useEffect, useState } from "react"
import { toast } from "sonner"

import { ChoiceSelect } from "@/components/choice-select"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import { cookieLinkSources, deleteCookieLink, saveCookieLink } from "@/lib/api/client"
import type { CookieLinkCopy } from "@/lib/api/types"

export function CookieSourceField({
  accountId,
  onApply,
}: {
  accountId: number
  onApply: (copied: CookieLinkCopy) => void
}) {
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [isSource, setIsSource] = useState(false)
  const [current, setCurrent] = useState<{ id: number; label: string } | null>(null)
  const [sources, setSources] = useState<Array<{ id: number; label: string }>>([])
  const [picked, setPicked] = useState("")

  useEffect(() => {
    let cancel = false
    cookieLinkSources(accountId)
      .then((data) => {
        if (cancel) return
        setIsSource(data.is_source)
        setSources(data.sources)
        setCurrent(
          data.current
            ? { id: data.current.source_account_id, label: data.current.source_label }
            : null,
        )
      })
      .catch((error: unknown) => {
        if (!cancel) toast.error(error instanceof Error ? error.message : "Could not load accounts")
      })
      .finally(() => {
        if (!cancel) setLoading(false)
      })
    return () => {
      cancel = true
    }
  }, [accountId])

  if (loading) {
    return (
      <FieldDescription className="flex items-center gap-2">
        <Spinner />
        Loading accounts…
      </FieldDescription>
    )
  }

  if (isSource) {
    return (
      <Field>
        <FieldLabel>Use from account</FieldLabel>
        <FieldDescription>Other accounts already use this cookie, so this one stays a source.</FieldDescription>
      </Field>
    )
  }

  return (
    <Field>
      <FieldLabel htmlFor="cookie-source">Use from account</FieldLabel>
      <FieldDescription>
        {current
          ? `Using ${current.label}. Later saves there replace this cookie until you stop.`
          : "Pick an account. Later saves there keep this cookie in sync until you stop."}
      </FieldDescription>
      <ChoiceSelect
        id="cookie-source"
        value={picked}
        onValueChange={setPicked}
        placeholder="Choose an account"
        items={sources.map((item) => ({ label: item.label, value: String(item.id) }))}
      />
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={!picked || busy}
          onClick={() => {
            setBusy(true)
            void saveCookieLink(accountId, Number(picked))
              .then((copied) => {
                const source = sources.find((item) => item.id === copied.source_account_id)
                setCurrent({
                  id: copied.source_account_id,
                  label: copied.source_label || source?.label || "that account",
                })
                setPicked("")
                onApply(copied)
                toast.success("Using that account")
              })
              .catch((error: unknown) => {
                toast.error(error instanceof Error ? error.message : "Could not use that account")
              })
              .finally(() => setBusy(false))
          }}
        >
          {busy ? <Spinner data-icon="inline-start" /> : null}
          Use
        </Button>
        {current ? (
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={() => {
              setBusy(true)
              void deleteCookieLink(accountId)
                .then(() => {
                  setCurrent(null)
                  toast.success("Stopped. This account keeps the cookie it has now.")
                })
                .catch((error: unknown) => {
                  toast.error(error instanceof Error ? error.message : "Could not stop")
                })
                .finally(() => setBusy(false))
            }}
          >
            Stop using
          </Button>
        ) : null}
      </div>
    </Field>
  )
}
