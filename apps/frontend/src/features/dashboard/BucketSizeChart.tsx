import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Pie, PieChart } from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  type ChartConfig,
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import { listBuckets } from "@/features/buckets/api";
import { bucketsKeys } from "@/lib/api/keys";

const TOP_N = 5;
const CHART_VARS = [
  "hsl(var(--chart-1))",
  "hsl(var(--chart-2))",
  "hsl(var(--chart-3))",
  "hsl(var(--chart-4))",
  "hsl(var(--chart-5))",
];

// Fetch enough buckets to chart meaningfully without unbounded fan-out; the
// remainder collapses into an "Other" slice.
const LIST_PARAMS = { page: 1, size: 200, sort: "-estimated_bytes" };

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

type Slice = { key: string; bytes: number; fill: string };

export function BucketSizeChart() {
  const q = useQuery({
    queryKey: bucketsKeys.list(LIST_PARAMS),
    queryFn: () => listBuckets(LIST_PARAMS),
  });

  const { slices, config } = useMemo(() => {
    const buckets = (q.data?.buckets ?? []).filter((b) => b.estimated_bytes > 0);
    const sorted = [...buckets].sort((a, b) => b.estimated_bytes - a.estimated_bytes);
    const top = sorted.slice(0, TOP_N);
    const rest = sorted.slice(TOP_N);

    const cfg: ChartConfig = {};
    const rows: Slice[] = top.map((b, i) => {
      const key = `b${i}`;
      cfg[key] = { label: b.name, color: CHART_VARS[i % CHART_VARS.length] };
      return { key, bytes: b.estimated_bytes, fill: `var(--color-${key})` };
    });
    const restBytes = rest.reduce((sum, b) => sum + b.estimated_bytes, 0);
    if (restBytes > 0) {
      cfg.other = { label: `Other (${rest.length})`, color: "hsl(var(--muted-foreground))" };
      rows.push({ key: "other", bytes: restBytes, fill: "var(--color-other)" });
    }
    return { slices: rows, config: cfg };
  }, [q.data]);

  return (
    <Card className="flex h-full flex-col">
      <CardHeader>
        <CardTitle>Storage by bucket</CardTitle>
      </CardHeader>
      <CardContent className="flex-1">
        {q.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : q.isError ? (
          <p className="text-sm text-destructive">Failed to load bucket sizes.</p>
        ) : slices.length === 0 ? (
          <p className="text-sm text-muted-foreground">No bucket usage to chart yet.</p>
        ) : (
          <ChartContainer config={config} className="mx-auto aspect-square max-h-[280px]">
            <PieChart>
              <ChartTooltip
                content={
                  <ChartTooltipContent
                    nameKey="key"
                    hideLabel
                    formatter={(value, name) => (
                      <div className="flex flex-1 items-center justify-between gap-3">
                        <span className="text-muted-foreground">
                          {config[name as string]?.label ?? name}
                        </span>
                        <span className="font-mono font-medium">{formatBytes(Number(value))}</span>
                      </div>
                    )}
                  />
                }
              />
              <Pie data={slices} dataKey="bytes" nameKey="key" />
              <ChartLegend content={<ChartLegendContent nameKey="key" />} className="flex-wrap" />
            </PieChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}
