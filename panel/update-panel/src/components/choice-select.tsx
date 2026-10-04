"use client"

import { useLayoutEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
  ComboboxValue,
} from "@/components/ui/combobox"

type ChoiceItem = { label: string; value: string }

export function ChoiceSelect({
  id,
  value,
  onValueChange,
  items,
  placeholder = "Select",
  invalid,
}: {
  id?: string
  value: string
  onValueChange: (value: string) => void
  items: ChoiceItem[]
  placeholder?: string
  invalid?: boolean
}) {
  const selected = items.find((item) => item.value === value) ?? null
  const rootRef = useRef<HTMLDivElement>(null)
  const [container, setContainer] = useState<HTMLElement | undefined>(undefined)

  useLayoutEffect(() => {
    const parent = rootRef.current?.closest("[data-slot='dialog-content'], [data-slot='sheet-content']")
    setContainer(parent instanceof HTMLElement ? parent : undefined)
  }, [])

  return (
    <div ref={rootRef} className="w-full">
    <Combobox
      items={items}
      value={selected}
      onValueChange={(next) => {
        if (next && typeof next === "object" && "value" in next) onValueChange(String(next.value))
      }}
      itemToStringLabel={(item) => item.label}
      itemToStringValue={(item) => item.label}
      isItemEqualToValue={(item, current) => item.value === current.value}
    >
      <ComboboxTrigger
        id={id}
        aria-invalid={invalid || undefined}
        className="w-full"
        render={<Button variant="outline" className="w-full justify-between font-normal" />}
      >
        <ComboboxValue placeholder={placeholder} />
      </ComboboxTrigger>
      <ComboboxContent container={container} className="max-h-80">
        <ComboboxInput placeholder="Search" showTrigger={false} />
        <ComboboxEmpty>No matches</ComboboxEmpty>
        <ComboboxList className="max-h-72">
          {(item: ChoiceItem) => (
            <ComboboxItem key={item.value} value={item}>
              {item.label}
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
    </div>
  )
}
