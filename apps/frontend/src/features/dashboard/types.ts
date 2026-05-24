export type DashboardWindow = "24h" | "7d" | "30d";

export type ServerInfo = {
  version: string;
  deployment_mode: string;
  uptime_seconds: number;
};

export type Totals = {
  buckets: number;
  estimated_bytes: number;
  objects: number;
};

export type NodeStatus = {
  endpoint: string;
  state: string;
  drives: { total: number; healthy: number; unhealthy: number };
};

export type EventSummary = {
  id: string;
  occurred_at: string;
  action: string;
  target_type: string;
  target_id: string;
  outcome: string;
};

export type FailureSummary = EventSummary & {
  source_ip: string;
  error_message: string;
};

export type DashboardView = {
  server: ServerInfo;
  totals: Totals;
  nodes: NodeStatus[];
  warnings: string[];
  recent_activity: EventSummary[];
  recent_failures: {
    window: DashboardWindow;
    count: number;
    entries: FailureSummary[];
  };
};
