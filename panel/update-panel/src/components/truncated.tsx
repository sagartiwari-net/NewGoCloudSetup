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
}: {
  value: string
  className?: string
}) {
  const text = value || "—"
  return (
    <Tooltip>
      <TooltipTrigger className={cn("block max-w-56 truncate text-left", className)}>
        {text}
      </TooltipTrigger>
      <TooltipContent>{text}</TooltipContent>
    </Tooltip>
  )
}
