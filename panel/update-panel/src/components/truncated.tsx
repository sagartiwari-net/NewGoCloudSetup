"use client"

import { cn } from "cn"

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"

export function Truncated({
  value,
  className,
  maxWidthClass = "max-w-56",
}: {
  value: string
  className?: string
  maxWidthClass?: string
}) {
  const text = value || "—"
  return (
    <Tooltip>
      <TooltipTrigger className={cn("block truncate text-left", maxWidthClass, className)}>
        {text}
      </TooltipTrigger>
      <TooltipContent className="max-w-sm break-words whitespace-pre-wrap">{text}</TooltipContent>
    </Tooltip>
  )
}
