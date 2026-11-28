"use client"

import { useState } from "react"

import { useAuth } from "@/components/auth-provider"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Spinner } from "@/components/ui/spinner"
import { useResource } from "@/hooks/use-resource"
import {
  downloadAhrefsBarExtension,
  downloadAhrefsBarTMCheck,
  getAhrefsBarExtInfo,
} from "@/lib/api/client"
import { runMutation } from "@/lib/mutate"

export default function AhrefsBarDownloadPage() {
  const { session } = useAuth()
  const { data, loading } = useResource(`${session?.role ?? "none"}-ext`, () => getAhrefsBarExtInfo())
  const [busyZip, setBusyZip] = useState(false)
  const [busyCheck, setBusyCheck] = useState(false)

  return (
    <>
      <PageHeader
        title="Ahrefs Bar download"
        description="Branded extension zip (API + portals stamped) and tm-check.php for aMember sites."
      />
      {loading && !data ? (
        <div className="flex justify-center py-16"><Spinner /></div>
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>Extension zip</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Yahan se naya branded zip nikaalo (API + portals stamp). Local test me Public API
                <code className="mx-1">http://127.0.0.1:8088</code> hona chahiye — agar
                <code className="mx-1">barapi.upays.in</code> dikhe to zip prod pe point karega aur
                Refresh pe <strong>no_portals</strong> aayega. Unzip → Load unpacked. Dev shortcut:
                seedha <code className="mx-1">ahrefs/extension</code> folder load karo.
              </p>
              <dl className="space-y-2 text-sm">
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Public API</dt>
                  <dd className="font-mono text-xs">{data?.public_api || "—"}</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Source</dt>
                  <dd className="text-right text-xs">{data?.extension_ok ? "ready" : "missing AHREFS_BAR_EXT_DIR"}</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-muted-foreground">Bound portals</dt>
                  <dd>{data?.portals?.length ?? 0}</dd>
                </div>
              </dl>
              {(data?.portals ?? []).length > 0 ? (
                <ul className="list-inside list-disc text-xs text-muted-foreground">
                  {data!.portals.map((p) => (
                    <li key={p} className="font-mono">{p}</li>
                  ))}
                </ul>
              ) : (
                <p className="text-xs text-muted-foreground">Add portals first so host permissions match your member sites.</p>
              )}
              <Button
                disabled={!data?.extension_ok || busyZip}
                onClick={() => {
                  setBusyZip(true)
                  void runMutation(() => downloadAhrefsBarExtension(), "Extension zip ready").finally(() => setBusyZip(false))
                }}
              >
                {busyZip ? <Spinner data-icon="inline-start" /> : null}
                Download extension.zip
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>tm-check.php</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Upload to the aMember site, set Portal → tm-check URL in Ahrefs Bar → Portals,
                and optionally match a shared secret.
              </p>
              <p className="text-xs text-muted-foreground">
                Template {data?.template_ok ? "ready" : "missing — set AHREFS_BAR_TEMPLATE_DIR"}
              </p>
              <Button
                variant="outline"
                disabled={!data?.template_ok || busyCheck}
                onClick={() => {
                  setBusyCheck(true)
                  void runMutation(() => downloadAhrefsBarTMCheck(), "tm-check.php ready").finally(() => setBusyCheck(false))
                }}
              >
                {busyCheck ? <Spinner data-icon="inline-start" /> : null}
                Download tm-check.php
              </Button>
            </CardContent>
          </Card>
        </div>
      )}
    </>
  )
}
