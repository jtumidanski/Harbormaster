export type MetricsWindow = "1h" | "6h" | "24h" | "7d";

export type MetricPoint = { t: string; v: number };

export type MetricsView = {
  window: string;
  step_seconds: number;
  collected: boolean;
  series: Record<string, MetricPoint[]>;
};
