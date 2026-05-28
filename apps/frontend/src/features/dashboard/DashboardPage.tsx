import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { capitalize } from "@/lib/utils";
import { AppError } from "@/lib/api/errors";
import { dashboardKeys } from "@/lib/api/keys";
import { fetchDashboard } from "./api";
import type { DashboardWindow, NodeStatus } from "./types";
import { BucketSizeChart } from "./BucketSizeChart";
import { RecentActivityList } from "./RecentActivityList";
import { RecentFailuresWidget } from "./RecentFailuresWidget";

const STORAGE_KEY = "harbormaster:dashboard:failuresWindow";

function isWindow(v: unknown): v is DashboardWindow {
  return v === "24h" || v === "7d" || v === "30d";
}

function readStoredWindow(): DashboardWindow {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    if (isWindow(v)) return v;
  } catch {
    // ignore
  }
  return "7d";
}

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

function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "0m";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0 || d > 0) parts.push(`${h}h`);
  parts.push(`${m}m`);
  return parts.join(" ");
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-4">
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold">{value}</p>
    </Card>
  );
}

function NodeCard({ node }: { node: NodeStatus }) {
  const healthy = node.drives.unhealthy === 0 && node.state.toLowerCase() === "online";
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between">
        <p className="truncate text-sm font-medium" title={node.endpoint}>
          {node.endpoint}
        </p>
        <Badge
          variant="outline"
          className={
            healthy
              ? "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/30 dark:text-emerald-200"
              : "bg-destructive/15 text-destructive"
          }
        >
          {capitalize(node.state)}
        </Badge>
      </div>
      <p className="mt-2 text-sm text-muted-foreground">
        Drives: {node.drives.healthy}/{node.drives.total} healthy
        {node.drives.unhealthy > 0 ? ` (${node.drives.unhealthy} unhealthy)` : ""}
      </p>
    </Card>
  );
}

export function DashboardPage() {
  const [failuresWindow, setFailuresWindowState] = useState<DashboardWindow>(() =>
    readStoredWindow(),
  );

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, failuresWindow);
    } catch {
      // ignore
    }
  }, [failuresWindow]);

  const q = useQuery({
    queryKey: dashboardKeys.view(failuresWindow),
    queryFn: () => fetchDashboard(failuresWindow),
  });

  if (q.isLoading) {
    return (
      <div className="p-6">
        <h1 className="mb-4 text-2xl font-semibold">Dashboard</h1>
        <p className="text-muted-foreground">Loading…</p>
      </div>
    );
  }

  if (q.isError || !q.data) {
    return (
      <div className="p-6">
        <h1 className="mb-4 text-2xl font-semibold">Dashboard</h1>
        <p className="text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load dashboard."}
        </p>
      </div>
    );
  }

  const view = q.data;

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-semibold">Dashboard</h1>

      <Card aria-label="Server" className="p-4">
        <h2 className="mb-2 text-lg font-semibold">Server</h2>
        <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground">Version</dt>
            <dd className="font-medium">{view.server.version}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Deployment mode</dt>
            <dd className="font-medium">{view.server.deployment_mode}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Uptime</dt>
            <dd className="font-medium">{formatUptime(view.server.uptime_seconds)}</dd>
          </div>
        </dl>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <section aria-label="Totals" className="grid grid-cols-1 gap-4">
          <StatCard label="Buckets" value={view.totals.buckets.toLocaleString()} />
          <StatCard label="Estimated size" value={formatBytes(view.totals.estimated_bytes)} />
          <StatCard label="Objects" value={view.totals.objects.toLocaleString()} />
        </section>
        <section aria-label="Storage by bucket">
          <BucketSizeChart />
        </section>
      </div>

      {view.warnings.length > 0 && (
        <Alert variant="destructive">
          <AlertTitle>Warnings</AlertTitle>
          <AlertDescription>
            <ul className="ml-4 list-disc">
              {view.warnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <section aria-label="Nodes">
        <h2 className="mb-2 text-lg font-semibold">Nodes</h2>
        {view.nodes.length === 0 ? (
          <p className="text-sm text-muted-foreground">No node information available.</p>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {view.nodes.map((n) => (
              <NodeCard key={n.endpoint} node={n} />
            ))}
          </div>
        )}
      </section>

      <section className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <RecentActivityList events={view.recent_activity} />
        <RecentFailuresWidget
          window={failuresWindow}
          onWindowChange={setFailuresWindowState}
          count={view.recent_failures.count}
          entries={view.recent_failures.entries}
        />
      </section>
    </div>
  );
}
