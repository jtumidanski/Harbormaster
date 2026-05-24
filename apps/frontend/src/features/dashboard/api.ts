import { api } from "@/lib/api/client";
import type { DashboardView, DashboardWindow } from "./types";

export function fetchDashboard(window: DashboardWindow): Promise<DashboardView> {
  return api.get<DashboardView>(`/api/v1/dashboard?failures_window=${window}`);
}
