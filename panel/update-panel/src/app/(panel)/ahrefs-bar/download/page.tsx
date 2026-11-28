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
        description="Branded extension zip and tm-check.php for aMember."
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
              <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
                <li>Add portals first (Permissions / member hosts).</li>
                <li>Download zip → unzip.</li>
                <li>
                  Chrome → Load unpacked → choose the unzipped folder.
                </li>
                <li>
                  Prod API is <span className="font-mono">barapi.upays.in</span>. Local test needs{" "}
                  <span className="font-mono">127.0.0.1:8088</span> stamped instead.
                </li>
              </ol>
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
                <p className="text-xs text-muted-foreground">No portals yet — zip host permissions will be empty.</p>
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
              <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
                <li>Download tm-check.php.</li>
                <li>Upload it to the aMember site.</li>
                <li>Portals → set tm-check URL (+ shared secret if used).</li>
              </ol>
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
