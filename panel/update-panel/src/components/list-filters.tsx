"use client"

import { ChoiceSelect } from "@/components/choice-select"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"

export function ListFilters({
  query,
  onQuery,
  searchId,
  placeholder,
  toolId,
  onTool,
  tools,
  resellerId,
  onReseller,
  resellers,
}: {
  query: string
  onQuery: (value: string) => void
  searchId: string
  placeholder: string
  toolId?: string
  onTool?: (value: string) => void
  tools?: Array<{ id: number; name: string }>
  resellerId?: string
  onReseller?: (value: string) => void
  resellers?: Array<{ id: number; username: string }>
}) {
  return (
    <div className="flex flex-col gap-3 md:flex-row md:items-end">
      <Field className="md:max-w-sm md:flex-1">
        <FieldLabel htmlFor={searchId}>Search</FieldLabel>
        <Input
          id={searchId}
          value={query}
          placeholder={placeholder}
          onChange={(event) => onQuery(event.target.value)}
        />
      </Field>
      {tools && onTool && toolId ? (
        <Field className="md:w-56">
          <FieldLabel htmlFor={`${searchId}-tool`}>Tool</FieldLabel>
          <ChoiceSelect
            id={`${searchId}-tool`}
            value={toolId}
            onValueChange={onTool}
            items={[
              { label: "All tools", value: "all" },
              ...tools.map((tool) => ({ label: tool.name, value: String(tool.id) })),
            ]}
          />
        </Field>
      ) : null}
      {resellers && onReseller && resellerId ? (
        <Field className="md:w-56">
          <FieldLabel htmlFor={`${searchId}-reseller`}>Reseller</FieldLabel>
          <ChoiceSelect
            id={`${searchId}-reseller`}
            value={resellerId}
            onValueChange={onReseller}
            items={[
              { label: "All resellers", value: "all" },
              ...resellers.map((reseller) => ({ label: reseller.username, value: String(reseller.id) })),
            ]}
          />
        </Field>
      ) : null}
    </div>
  )
}
