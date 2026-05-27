export type AuditEvent = {
  id: string;
  occurred_at: string;
  actor: string;
  source_ip: string;
  action: string;
  target_type: string;
  target_id: string;
  outcome: string;
  error_message: string | null;
  payload_summary: Record<string, unknown>;
};

export type AuditFilter = {
  action?: string;
  target_type?: string;
  target_id?: string;
  outcome?: string;
  from?: string;
  to?: string;
};

export type AuditPage = { number: number; size: number };

export type AuditCollectionResponse = {
  data: Array<{ type: "audit_events"; id: string; attributes: AuditEvent }>;
  meta?: {
    page?: { number: number; size: number; total_pages: number; total_records: number };
  };
};
