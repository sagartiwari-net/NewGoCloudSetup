import { Badge } from "@/components/ui/badge"

export function StatusBadge({ status }: { status: string }) {
  const variant =
    status === "active" || status === "saved"
      ? "default"
      : status === "failed" || status === "suspended" || status === "inactive" || status === "logged_out" || status === "blocked"
        ? "destructive"
        : "secondary"
  return <Badge variant={variant}>{status}</Badge>
}
