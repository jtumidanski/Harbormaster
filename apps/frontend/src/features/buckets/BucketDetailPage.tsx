import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AppError } from "@/lib/api/errors";
import { bucketsKeys } from "@/lib/api/keys";
import { LifecycleRulesTab } from "@/features/lifecycle/LifecycleRulesTab";
import { ObjectBrowserPage } from "@/features/objects/ObjectBrowserPage";
import { getBucket, setBucketVersioning } from "./api";
import type { Bucket, PublicAccess } from "./types";
import { DeleteBucketDialog } from "./DeleteBucketDialog";
import { EditPublicAccessDialog } from "./EditPublicAccessDialog";
import { EditQuotaDialog } from "./EditQuotaDialog";
import { EmptyBucketDialog } from "./EmptyBucketDialog";

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let i = 0;
  let n = bytes;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n >= 10 || i === 0 ? n.toFixed(0) : n.toFixed(1)} ${units[i]}`;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function publicAccessLabel(mode: PublicAccess): string {
  switch (mode) {
    case "private":
      return "Private";
    case "public-read":
      return "Public read";
    case "public-read-write":
      return "Public RW";
  }
}

function publicAccessBadgeClass(mode: PublicAccess): string {
  switch (mode) {
    case "private":
      return "bg-muted text-muted-foreground";
    case "public-read":
      return "bg-amber-100 text-amber-900 dark:bg-amber-900/30 dark:text-amber-200";
    case "public-read-write":
      return "bg-destructive/15 text-destructive";
  }
}

function QuotaBar({ bucket }: { bucket: Bucket }) {
  if (!bucket.quota) {
    return <div className="text-sm text-muted-foreground">No quota set.</div>;
  }
  const pct =
    bucket.quota.bytes > 0
      ? Math.min(100, Math.round((bucket.quota.used_bytes / bucket.quota.bytes) * 100))
      : 0;
  const tone = pct >= 95 ? "bg-destructive" : pct >= 80 ? "bg-amber-500" : "bg-primary";
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between text-sm">
        <span>
          {formatBytes(bucket.quota.used_bytes)} of {formatBytes(bucket.quota.bytes)} used
          <span className="ml-2 text-xs uppercase text-muted-foreground">
            ({bucket.quota.kind})
          </span>
        </span>
        <span className="text-xs text-muted-foreground">{pct}%</span>
      </div>
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        className="h-2 w-full overflow-hidden rounded bg-muted"
      >
        <div className={`h-full ${tone} transition-all`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

export function BucketDetailPage() {
  const { name = "" } = useParams<{ name: string }>();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: bucketsKeys.detail(name),
    queryFn: () => getBucket(name),
    enabled: name.length > 0,
  });

  const [publicAccessOpen, setPublicAccessOpen] = useState(false);
  const [quotaOpen, setQuotaOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [emptyOpen, setEmptyOpen] = useState(false);

  const versioningMutation = useMutation({
    mutationFn: (enabled: boolean) => setBucketVersioning(name, enabled),
    onMutate: async (enabled: boolean) => {
      await qc.cancelQueries({ queryKey: bucketsKeys.detail(name) });
      const prev = qc.getQueryData<Bucket>(bucketsKeys.detail(name));
      if (prev) {
        qc.setQueryData<Bucket>(bucketsKeys.detail(name), {
          ...prev,
          versioning_enabled: enabled,
        });
      }
      return { prev };
    },
    onError: (err: unknown, _enabled, ctx) => {
      if (ctx?.prev) qc.setQueryData(bucketsKeys.detail(name), ctx.prev);
      if (err instanceof AppError) {
        toast.error(err.message || "Failed to update versioning.");
      } else {
        toast.error("Failed to update versioning.");
      }
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: bucketsKeys.detail(name) });
      await qc.invalidateQueries({ queryKey: bucketsKeys.all() });
      toast.success("Versioning updated.");
    },
  });

  if (!name) return <div className="p-6">Missing bucket name.</div>;
  if (q.isLoading) return <div className="p-6 text-muted-foreground">Loading…</div>;
  if (q.isError || !q.data) {
    return (
      <div className="p-6 space-y-3">
        <p className="text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load bucket."}
        </p>
        <Button variant="outline" size="sm" asChild>
          <Link to="/buckets">
            <ArrowLeft aria-hidden="true" /> Back to buckets
          </Link>
        </Button>
      </div>
    );
  }

  const bucket = q.data;
  const hasObjects = bucket.object_count > 0;

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <Button variant="ghost" size="sm" asChild className="-ml-2 text-muted-foreground">
            <Link to="/buckets">
              <ArrowLeft aria-hidden="true" /> Buckets
            </Link>
          </Button>
          <h1 className="text-2xl font-semibold">{bucket.name}</h1>
        </div>
        <Badge variant="outline" className={publicAccessBadgeClass(bucket.public_access)}>
          {publicAccessLabel(bucket.public_access)}
        </Badge>
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="objects">Objects</TabsTrigger>
          <TabsTrigger value="lifecycle">Lifecycle</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Overview</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div>
                <div className="text-xs uppercase text-muted-foreground">Created</div>
                <div>{formatDate(bucket.created_at)}</div>
              </div>
              <div>
                <div className="text-xs uppercase text-muted-foreground">Estimated size</div>
                <div>{formatBytes(bucket.estimated_bytes)}</div>
              </div>
              <div>
                <div className="text-xs uppercase text-muted-foreground">Object count</div>
                <div>{bucket.object_count.toLocaleString()}</div>
              </div>
              <div>
                <div className="text-xs uppercase text-muted-foreground">Lifecycle rules</div>
                <div>{bucket.has_lifecycle_rules ? "Configured" : "None"}</div>
              </div>
              <div className="sm:col-span-2 flex items-center gap-2">
                <input
                  id="bucket-versioning"
                  type="checkbox"
                  className="h-4 w-4"
                  checked={bucket.versioning_enabled}
                  disabled={versioningMutation.isPending}
                  onChange={(e) => versioningMutation.mutate(e.target.checked)}
                />
                <Label htmlFor="bucket-versioning" className="font-normal">
                  Versioning {bucket.versioning_enabled ? "enabled" : "disabled"}
                </Label>
              </div>
              <div className="sm:col-span-2">
                <div className="mb-1 text-xs uppercase text-muted-foreground">Quota</div>
                <QuotaBar bucket={bucket} />
              </div>
            </CardContent>
          </Card>

          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => setPublicAccessOpen(true)}>
              Edit public access
            </Button>
            <Button variant="outline" onClick={() => setQuotaOpen(true)}>
              Edit quota
            </Button>
            <Button variant="outline" onClick={() => setEmptyOpen(true)}>
              Empty bucket
            </Button>
            <div className="flex items-center gap-2">
              <Button
                variant="destructive"
                disabled={hasObjects}
                onClick={() => setDeleteOpen(true)}
              >
                Delete bucket
              </Button>
              {hasObjects && (
                <button
                  type="button"
                  className="text-sm text-primary hover:underline"
                  onClick={() => setEmptyOpen(true)}
                >
                  Empty this bucket first
                </button>
              )}
            </div>
          </div>
        </TabsContent>

        <TabsContent value="objects">
          <ObjectBrowserPage bucket={bucket.name} />
        </TabsContent>

        <TabsContent value="lifecycle">
          <LifecycleRulesTab bucket={bucket.name} versioningEnabled={bucket.versioning_enabled} />
        </TabsContent>
      </Tabs>

      <EditPublicAccessDialog
        open={publicAccessOpen}
        onOpenChange={setPublicAccessOpen}
        bucketName={bucket.name}
        currentMode={bucket.public_access}
      />
      <EditQuotaDialog
        open={quotaOpen}
        onOpenChange={setQuotaOpen}
        bucketName={bucket.name}
        currentQuota={bucket.quota}
        versioningEnabled={bucket.versioning_enabled}
      />
      <DeleteBucketDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        bucketName={bucket.name}
        objectCount={bucket.object_count}
        onEmptyFirst={() => setEmptyOpen(true)}
      />
      <EmptyBucketDialog
        open={emptyOpen}
        onOpenChange={setEmptyOpen}
        bucketName={bucket.name}
        versioningEnabled={bucket.versioning_enabled}
        estimatedTotal={bucket.object_count}
      />
    </div>
  );
}
