import { api } from "@/lib/api/client";
import type { MetricsView, MetricsWindow } from "./types";

export async function fetchMetrics(window: MetricsWindow): Promise<MetricsView> {
  return api.get<MetricsView>(`/api/v1/metrics?window=${encodeURIComponent(window)}`);
}
