"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { toast } from "sonner"

export function useResource<T>(key: string, load: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [nonce, setNonce] = useState(0)
  const [seenKey, setSeenKey] = useState(key)
  const loadRef = useRef(load)

  if (seenKey !== key) {
    setSeenKey(key)
    setLoading(true)
  }

  const reload = useCallback(() => {
    setLoading(true)
    setNonce((current) => current + 1)
  }, [])

  useEffect(() => {
    loadRef.current = load
  }, [load])

  useEffect(() => {
    let active = true
    loadRef
      .current()
      .then((value) => {
        if (active) setData(value)
      })
      .catch((error: unknown) => {
        if (active) {
          toast.error(error instanceof Error ? error.message : "Could not load")
        }
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [key, nonce])

  return { data, loading, reload }
}
