"use client"

import { useEffect, useState } from "react"
import { toast } from "sonner"

import { ChoiceSelect } from "@/components/choice-select"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import {
  ahrefsBarAccountLinkSources,
  deleteAhrefsBarAccountLink,
  saveAhrefsBarAccountLink,
} from "@/lib/api/client"
import type { AhrefsBarAccountLinkCopy } from "@/lib/api/types"

/** Master-only: pool this Ahrefs Bar account's token from another account (like Shared cookies). */
export function AhrefsBarTokenSourceField({
  accountId,
  onApply,
}: {
  accountId: number
  onApply: (copied: AhrefsBarAccountLinkCopy) => void
}) {
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [isSource, setIsSource] = useState(false)
  const [current, setCurrent] = useState<{ id: number; label: string } | null>(null)
  const [sources, setSources] = useState<Array<{ id: number; label: string }>>([])
  const [picked, setPicked] = useState("")

  useEffect(() => {
    let cancel = false
    ahrefsBarAccountLinkSources(accountId)
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
        Loading pool options…
      </FieldDescription>
    )
  }

  if (isSource) {
    return (
      <Field>
        <FieldLabel>Pool from account</FieldLabel>
        <FieldDescription>Other accounts already use this token, so this one stays a source.</FieldDescription>
      </Field>
    )
  }

  return (
    <Field>
      <FieldLabel htmlFor="ab-token-source">Pool from account</FieldLabel>
      <FieldDescription>
        {current
          ? `Using ${current.label}. When that token is updated, this one updates too.`
          : "Pick an account. Later token saves there sync here until you stop."}
      </FieldDescription>
      <ChoiceSelect
        id="ab-token-source"
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
            void saveAhrefsBarAccountLink(accountId, Number(picked))
              .then((copied) => {
                const source = sources.find((item) => item.id === copied.source_account_id)
                setCurrent({
                  id: copied.source_account_id,
                  label: copied.source_label || source?.label || "that account",
                })
                setPicked("")
                onApply(copied)
                toast.success("Pooled from that account")
              })
              .catch((error: unknown) => {
                toast.error(error instanceof Error ? error.message : "Could not pool")
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
              void deleteAhrefsBarAccountLink(accountId)
                .then(() => {
                  setCurrent(null)
                  toast.success("Stopped pooling. This account keeps its current token.")
                })
                .catch((error: unknown) => {
                  toast.error(error instanceof Error ? error.message : "Could not stop")
                })
                .finally(() => setBusy(false))
            }}
          >
            Stop pooling
          </Button>
        ) : null}
      </div>
    </Field>
  )
}
