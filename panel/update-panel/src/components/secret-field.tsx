"use client"

import { useState, type CSSProperties } from "react"
import { EyeIcon, EyeOffIcon } from "lucide-react"

import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"

export function SecretTextarea({
  id,
  value,
  onChange,
  invalid,
}: {
  id: string
  value: string
  onChange: (value: string) => void
  invalid?: boolean
}) {
  const [revealed, setRevealed] = useState(!value)

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => setRevealed((current) => !current)}
        >
          {revealed ? <EyeOffIcon data-icon="inline-start" /> : <EyeIcon data-icon="inline-start" />}
          {revealed ? "Hide" : "Reveal"}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            if (!value) {
              toast.error("Nothing to copy")
              return
            }
            void navigator.clipboard.writeText(value).then(
              () => toast.success("Copied"),
              () => toast.error("Could not copy"),
            )
          }}
        >
          Copy
        </Button>
        <Button type="button" variant="outline" onClick={() => onChange("")}>
          Clear
        </Button>
      </div>
      <Textarea
        id={id}
        value={value}
        aria-invalid={invalid || undefined}
        onFocus={() => setRevealed(true)}
        onChange={(event) => onChange(event.target.value)}
        className="h-28 resize-none overflow-y-auto font-mono"
        style={{
          fieldSizing: "fixed",
          WebkitTextSecurity: revealed ? "none" : "disc",
        } as CSSProperties}
        placeholder="Paste the cookie JSON here"
      />
    </div>
  )
}
