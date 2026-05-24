import { api } from "@/lib/api/client";
import type { AuditCollectionResponse, AuditFilter, AuditPage } from "./types";

export function listAuditEvents(
  filter: AuditFilter,
  page: AuditPage,
): Promise<AuditCollectionResponse> {
  const sp = new URLSearchParams();
  if (filter.action) sp.set("filter[action]", filter.action);
  if (filter.target_type) sp.set("filter[target_type]", filter.target_type);
  if (filter.target_id) sp.set("filter[target_id]", filter.target_id);
  if (filter.outcome) sp.set("filter[outcome]", filter.outcome);
  if (filter.from) sp.set("filter[from]", filter.from);
  if (filter.to) sp.set("filter[to]", filter.to);
  sp.set("page[number]", String(page.number));
  sp.set("page[size]", String(page.size));
  return api.get<AuditCollectionResponse>(`/api/v1/audit-events?${sp.toString()}`);
}
