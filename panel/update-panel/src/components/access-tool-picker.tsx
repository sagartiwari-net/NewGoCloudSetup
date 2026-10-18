"use client"

import { useState } from "react"

import { ChoiceSelect } from "@/components/choice-select"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldDescription, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useResource } from "@/hooks/use-resource"
import { listResellers } from "@/lib/api/client"
import type { Website } from "@/lib/api/types"

export function AccessToolPicker({
  websites,
  selected,
  onChange,
}: {
  websites: Website[]
  selected: number[]
  onChange: (websiteIds: number[]) => void
}) {
  const { data } = useResource("access-reseller-filter", () =>
    listResellers({ page: 1, pageSize: 200, query: "", status: "all" }),
  )
  const [resellerId, setResellerId] = useState("all")
  const [picked, setPicked] = useState("all")
  const [websiteQuery, setWebsiteQuery] = useState("")
  const resellers = data?.items ?? []
  const reseller = resellers.find((item) => String(item.id) === resellerId)
  const chosen = new Set(selected)
  const needle = websiteQuery.trim().toLowerCase()
  const shown = websites.filter((website) => {
    if (reseller && !reseller.website_ids.includes(website.id)) return false
    if (picked === "allowed" && !chosen.has(website.id)) return false
    if (picked === "not" && chosen.has(website.id)) return false
    if (!needle) return true
    return (
      website.name.toLowerCase().includes(needle) ||
      website.domain.toLowerCase().includes(needle) ||
      (website.access_slug ?? "").toLowerCase().includes(needle)
    )
  })

  function selectShown() {
    const next = new Set(selected)
    for (const website of shown) next.add(website.id)
    onChange([...next])
  }

  return (
    <FieldSet>
      <FieldLegend variant="label">Tools</FieldLegend>
      <FieldDescription>
        {selected.length} selected. Allowed is already checked. Not allowed is the rest. Select shown adds this list and keeps the others.
      </FieldDescription>
      <div className="flex flex-col gap-2">
        <div className="grid grid-cols-2 gap-2">
        <div className="min-w-0">
          <ChoiceSelect
            id="access-picked"
            value={picked}
            onValueChange={setPicked}
            items={[
              { label: "All", value: "all" },
              { label: "Allowed", value: "allowed" },
              { label: "Not allowed", value: "not" },
            ]}
          />
        </div>
        <div className="min-w-0">
          <ChoiceSelect
            id="access-reseller"
            value={resellerId}
            onValueChange={setResellerId}
            placeholder="All resellers"
            items={[
              { label: "All resellers", value: "all" },
              ...resellers.map((item) => ({ label: item.username, value: String(item.id) })),
            ]}
          />
        </div>
        </div>
        <div className="flex gap-2">
          <Input
            value={websiteQuery}
            placeholder="Search tools"
            className="min-w-0 flex-1"
            onChange={(event) => setWebsiteQuery(event.target.value)}
          />
          <Button type="button" variant="outline" onClick={selectShown} disabled={shown.length === 0}>
            Select shown
          </Button>
        </div>
      </div>
      <FieldGroup className="max-h-72 overflow-y-auto">
        {shown.map((website) => {
          const checked = selected.includes(website.id)
          const label = website.access_slug ? `${website.name} · ${website.access_slug}` : website.name
          return (
            <Field key={website.id} orientation="horizontal">
              <Checkbox
                id={`access-tool-${website.id}`}
                checked={checked}
                onCheckedChange={(next) => {
                  const ids = new Set(selected)
                  if (next) ids.add(website.id)
                  else ids.delete(website.id)
                  onChange([...ids])
                }}
              />
              <FieldLabel htmlFor={`access-tool-${website.id}`}>{label}</FieldLabel>
            </Field>
          )
        })}
        {shown.length === 0 ? <p className="text-sm text-muted-foreground">No tool matches.</p> : null}
      </FieldGroup>
    </FieldSet>
  )
}
