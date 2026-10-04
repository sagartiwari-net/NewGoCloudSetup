"use client"

import { useRef } from "react"

import { gsap, prefersReducedMotion, useGSAP } from "@/lib/gsap"

export function CountUp({ value }: { value: number }) {
  const ref = useRef<HTMLSpanElement>(null)

  useGSAP(
    () => {
      const node = ref.current
      if (!node) return
      if (prefersReducedMotion()) {
        node.textContent = value.toLocaleString()
        return
      }
      const state = { n: 0 }
      gsap.to(state, {
        n: value,
        duration: 0.8,
        ease: "power2.out",
        onUpdate: () => {
          node.textContent = Math.round(state.n).toLocaleString()
        },
      })
    },
    { scope: ref, dependencies: [value], revertOnUpdate: true }
  )

  return <span ref={ref}>{value.toLocaleString()}</span>
}
