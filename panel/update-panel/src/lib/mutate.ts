"use client"

import { toast } from "sonner"

export async function runMutation(action: () => Promise<unknown>, success: string) {
  try {
    await action()
    toast.success(success)
    return true
  } catch (error) {
    toast.error(error instanceof Error ? error.message : "Something went wrong")
    return false
  }
}
