"use client"

import { useAuth } from "@/components/auth-provider"
import { PageHeader } from "@/components/page-header"
import { Card, CardContent } from "@/components/ui/card"

export default function ProfilePage() {
  const { session } = useAuth()
  return (
    <>
      <PageHeader title="Profile" description={`Signed in as ${session?.username ?? "operator"}.`} />
      <Card className="max-w-lg">
        <CardContent className="text-sm text-muted-foreground">
          Role: {session?.role ?? "unknown"}
        </CardContent>
      </Card>
    </>
  )
}
