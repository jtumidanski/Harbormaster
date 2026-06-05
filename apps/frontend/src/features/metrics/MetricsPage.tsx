import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  type ChartConfig,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import { AppError } from "@/lib/api/errors";
import { metricsKeys } from "@/lib/api/keys";
import { fetchMetrics } from "./api";
import type { MetricPoint, MetricsView, MetricsWindow } from "./types";

const STORAGE_KEY = "harbormaster:metrics:window";
const ALLOWED_WINDOWS: MetricsWindow[] = ["1h", "6h", "24h", "7d"];

const WINDOW_LABELS: Record<MetricsWindow, string> = {
  "1h": "Last 1 hour",
  "6h": "Last 6 hours",
  "24h": "Last 24 hours",
  "7d": "Last 7 days",
};

function isMetricsWindow(v: unknown): v is MetricsWindow {
  return ALLOWED_WINDOWS.includes(v as MetricsWindow);
}

function readStoredWindow(): MetricsWindow {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    if (isMetricsWindow(v)) return v;
  } catch {
    // ignore
  }
  return "24h";
}

// Format timestamp for X axis
function formatTime(isoString: string, metricsWindow: MetricsWindow): string {
  try {
    const d = new Date(isoString);
    if (metricsWindow === "7d") {
      return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
    }
    return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  } catch {
    return isoString;
  }
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

function formatRate(v: number): string {
  if (!Number.isFinite(v)) return "0/s";
  return `${v.toFixed(2)}/s`;
}

// Convert MetricPoint[] to recharts datum shape
function toRechartsDatum(
  points: MetricPoint[],
  key: string,
  metricsWindow: MetricsWindow,
): Array<Record<string, string | number>> {
  return points.map((p) => ({
    t: formatTime(p.t, metricsWindow),
    [key]: p.v,
  }));
}

// Merge two series arrays into single recharts data aligned by formatted time
function mergeSeries(
  aPoints: MetricPoint[],
  aKey: string,
  bPoints: MetricPoint[],
  bKey: string,
  metricsWindow: MetricsWindow,
): Array<Record<string, string | number>> {
  const map = new Map<string, Record<string, string | number>>();
  for (const p of aPoints) {
    const t = formatTime(p.t, metricsWindow);
    map.set(t, { t, [aKey]: p.v });
  }
  for (const p of bPoints) {
    const t = formatTime(p.t, metricsWindow);
    const existing = map.get(t) ?? { t };
    map.set(t, { ...existing, [bKey]: p.v });
  }
  return Array.from(map.values()).sort((a, b) =>
    String(a.t) < String(b.t) ? -1 : String(a.t) > String(b.t) ? 1 : 0,
  );
}

// ── Individual chart components ──────────────────────────────────────────────

function RequestRateChart({
  series,
  metricsWindow,
}: {
  series: MetricsView["series"];
  metricsWindow: MetricsWindow;
}) {
  const points = series["minio_s3_requests_total"] ?? [];
  const data = toRechartsDatum(points, "requests", metricsWindow);

  const config: ChartConfig = {
    requests: { label: "Requests/s", color: "hsl(var(--chart-1))" },
  };

  return (
    <Card data-testid="metrics-chart-requests">
      <CardHeader>
        <CardTitle>Request Rate</CardTitle>
      </CardHeader>
      <CardContent>
        {data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No data available.</p>
        ) : (
          <ChartContainer config={config} className="h-[200px] w-full">
            <AreaChart data={data}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="t" tick={{ fontSize: 11 }} interval="preserveStartEnd" />
              <YAxis tickFormatter={(v: number) => formatRate(v)} width={60} />
              <ChartTooltip content={<ChartTooltipContent />} />
              <Area
                type="monotone"
                dataKey="requests"
                fill="var(--color-requests)"
                stroke="var(--color-requests)"
                fillOpacity={0.3}
              />
            </AreaChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}

function ErrorRatesChart({
  series,
  metricsWindow,
}: {
  series: MetricsView["series"];
  metricsWindow: MetricsWindow;
}) {
  const errors4xx = series["minio_s3_requests_4xx_errors_total"] ?? [];
  const errors5xx = series["minio_s3_requests_5xx_errors_total"] ?? [];
  const data = mergeSeries(errors4xx, "errors4xx", errors5xx, "errors5xx", metricsWindow);

  const config: ChartConfig = {
    errors4xx: { label: "4xx Errors/s", color: "hsl(var(--chart-4))" },
    errors5xx: { label: "5xx Errors/s", color: "hsl(var(--chart-5))" },
  };

  return (
    <Card data-testid="metrics-chart-errors">
      <CardHeader>
        <CardTitle>Error Rates</CardTitle>
      </CardHeader>
      <CardContent>
        {data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No data available.</p>
        ) : (
          <ChartContainer config={config} className="h-[200px] w-full">
            <LineChart data={data}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="t" tick={{ fontSize: 11 }} interval="preserveStartEnd" />
              <YAxis tickFormatter={(v: number) => formatRate(v)} width={60} />
              <ChartTooltip content={<ChartTooltipContent />} />
              <ChartLegend content={<ChartLegendContent />} />
              <Line
                type="monotone"
                dataKey="errors4xx"
                stroke="var(--color-errors4xx)"
                dot={false}
              />
              <Line
                type="monotone"
                dataKey="errors5xx"
                stroke="var(--color-errors5xx)"
                dot={false}
              />
            </LineChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}

function ThroughputChart({
  series,
  metricsWindow,
}: {
  series: MetricsView["series"];
  metricsWindow: MetricsWindow;
}) {
  const received = series["minio_s3_traffic_received_bytes"] ?? [];
  const sent = series["minio_s3_traffic_sent_bytes"] ?? [];
  const data = mergeSeries(received, "received", sent, "sent", metricsWindow);

  const config: ChartConfig = {
    received: { label: "Received/s", color: "hsl(var(--chart-2))" },
    sent: { label: "Sent/s", color: "hsl(var(--chart-3))" },
  };

  return (
    <Card data-testid="metrics-chart-throughput">
      <CardHeader>
        <CardTitle>Throughput</CardTitle>
      </CardHeader>
      <CardContent>
        {data.length === 0 ? (
          <p className="text-sm text-muted-foreground">No data available.</p>
        ) : (
          <ChartContainer config={config} className="h-[200px] w-full">
            <AreaChart data={data}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="t" tick={{ fontSize: 11 }} interval="preserveStartEnd" />
              <YAxis tickFormatter={(v: number) => formatBytes(v)} width={70} />
              <ChartTooltip content={<ChartTooltipContent />} />
              <ChartLegend content={<ChartLegendContent />} />
              <Area
                type="monotone"
                dataKey="received"
                fill="var(--color-received)"
                stroke="var(--color-received)"
                fillOpacity={0.3}
              />
              <Area
                type="monotone"
                dataKey="sent"
                fill="var(--color-sent)"
                stroke="var(--color-sent)"
                fillOpacity={0.3}
              />
            </AreaChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}

function CapacityAndDrivesChart({
  series,
  metricsWindow,
}: {
  series: MetricsView["series"];
  metricsWindow: MetricsWindow;
}) {
  const total = series["minio_cluster_capacity_usable_total_bytes"] ?? [];
  const free = series["minio_cluster_capacity_usable_free_bytes"] ?? [];
  const online = series["minio_cluster_drive_online_total"] ?? [];
  const offline = series["minio_cluster_drive_offline_total"] ?? [];

  const capacityData = mergeSeries(total, "total", free, "free", metricsWindow);
  const driveData = mergeSeries(online, "online", offline, "offline", metricsWindow);

  const capacityConfig: ChartConfig = {
    total: { label: "Total Capacity", color: "hsl(var(--chart-1))" },
    free: { label: "Free Capacity", color: "hsl(var(--chart-2))" },
  };

  const driveConfig: ChartConfig = {
    online: { label: "Online Drives", color: "hsl(var(--chart-2))" },
    offline: { label: "Offline Drives", color: "hsl(var(--chart-5))" },
  };

  return (
    <Card data-testid="metrics-chart-capacity" className="col-span-full">
      <CardHeader>
        <CardTitle>Cluster — Capacity &amp; Drives</CardTitle>
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div>
          <p className="mb-2 text-sm font-medium text-muted-foreground">Usable Capacity</p>
          {capacityData.length === 0 ? (
            <p className="text-sm text-muted-foreground">No data available.</p>
          ) : (
            <ChartContainer config={capacityConfig} className="h-[180px] w-full">
              <AreaChart data={capacityData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="t" tick={{ fontSize: 11 }} interval="preserveStartEnd" />
                <YAxis tickFormatter={(v: number) => formatBytes(v)} width={70} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <ChartLegend content={<ChartLegendContent />} />
                <Area
                  type="monotone"
                  dataKey="total"
                  fill="var(--color-total)"
                  stroke="var(--color-total)"
                  fillOpacity={0.2}
                />
                <Area
                  type="monotone"
                  dataKey="free"
                  fill="var(--color-free)"
                  stroke="var(--color-free)"
                  fillOpacity={0.3}
                />
              </AreaChart>
            </ChartContainer>
          )}
        </div>
        <div>
          <p className="mb-2 text-sm font-medium text-muted-foreground">Drive Status</p>
          {driveData.length === 0 ? (
            <p className="text-sm text-muted-foreground">No data available.</p>
          ) : (
            <ChartContainer config={driveConfig} className="h-[180px] w-full">
              <LineChart data={driveData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="t" tick={{ fontSize: 11 }} interval="preserveStartEnd" />
                <YAxis allowDecimals={false} width={40} />
                <ChartTooltip content={<ChartTooltipContent />} />
                <ChartLegend content={<ChartLegendContent />} />
                <Line type="monotone" dataKey="online" stroke="var(--color-online)" dot={false} />
                <Line type="monotone" dataKey="offline" stroke="var(--color-offline)" dot={false} />
              </LineChart>
            </ChartContainer>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export function MetricsPage() {
  const [metricsWindow, setMetricsWindowState] = useState<MetricsWindow>(() => readStoredWindow());

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, metricsWindow);
    } catch {
      // ignore
    }
  }, [metricsWindow]);

  const q = useQuery({
    queryKey: metricsKeys.view(metricsWindow),
    queryFn: () => fetchMetrics(metricsWindow),
    refetchInterval: 30_000,
  });

  if (q.isLoading) {
    return (
      <div className="p-6">
        <h1 className="mb-4 text-2xl font-semibold">Metrics</h1>
        <p className="text-muted-foreground">Loading…</p>
      </div>
    );
  }

  if (q.isError || !q.data) {
    return (
      <div className="p-6">
        <h1 className="mb-4 text-2xl font-semibold">Metrics</h1>
        <p className="text-destructive">
          {q.error instanceof AppError ? q.error.message : "Failed to load metrics."}
        </p>
      </div>
    );
  }

  const view = q.data;

  return (
    <div className="space-y-6 p-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h1 className="text-2xl font-semibold">Metrics</h1>
        <Select
          value={metricsWindow}
          onValueChange={(v) => {
            if (isMetricsWindow(v)) setMetricsWindowState(v);
          }}
        >
          <SelectTrigger className="w-44" aria-label="Metrics window">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {ALLOWED_WINDOWS.map((w) => (
              <SelectItem key={w} value={w}>
                {WINDOW_LABELS[w]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {!view.collected ? (
        <Alert>
          <AlertTitle>Metrics collection paused</AlertTitle>
          <AlertDescription>
            No recent samples. This is normal on a fresh install or when MinIO is unreachable.
          </AlertDescription>
        </Alert>
      ) : (
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <RequestRateChart series={view.series} metricsWindow={metricsWindow} />
          <ErrorRatesChart series={view.series} metricsWindow={metricsWindow} />
          <ThroughputChart series={view.series} metricsWindow={metricsWindow} />
          <CapacityAndDrivesChart series={view.series} metricsWindow={metricsWindow} />
        </div>
      )}
    </div>
  );
}
