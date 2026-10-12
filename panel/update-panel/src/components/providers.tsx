"use client"

import { useEffect } from "react"
import { ThemeProvider } from "next-themes"

import { AuthProvider } from "@/components/auth-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import "@/lib/gsap"

export function Providers({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    const url = new URL(window.location.href)
    if (url.searchParams.has("__v")) {
      url.searchParams.delete("__v")
      window.history.replaceState(null, "", url.pathname + url.search + url.hash)
      return
    }
    try {
      sessionStorage.removeItem("panel-static-reload")
    } catch {
      // ignore
    }
  }, [])

  return (
    <ThemeProvider attribute="class" defaultTheme="light" enableSystem={false}>
      <TooltipProvider>
        <AuthProvider>
          {children}
          <Toaster position="top-right" />
        </AuthProvider>
      </TooltipProvider>
    </ThemeProvider>
  )
}
