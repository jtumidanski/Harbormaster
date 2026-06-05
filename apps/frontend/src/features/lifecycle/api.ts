import { api } from "@/lib/api/client";

// LifecycleRule mirrors the backend's discriminated union: managed rules
// carry kind-specific fields while unmanaged rules expose only a human-readable
// summary string. Both variants share the `managed` flag.
export type LifecycleRule = {
  id: string;
  managed: boolean;
  kind?: string;
  days?: number;
  noncurrent_days?: number;
  newer_noncurrent_versions?: number;
  days_after_initiation?: number;
  prefix?: string;
  summary?: string;
};

// Discriminated union of create-rule attributes, one variant per backend kind.
export type CreateRuleAttrs =
  | { kind: "expiration"; days: number; prefix: string }
  | {
      kind: "noncurrent-expiration";
      noncurrent_days: number;
      newer_noncurrent_versions: number;
      prefix: string;
    }
  | { kind: "abort-incomplete-multipart"; days_after_initiation: number; prefix: string };

export type LifecycleListResponse = {
  data: Array<{ type: "lifecycle_rules"; id: string; attributes: LifecycleRule }>;
};

export type LifecycleSingleResponse = {
  data: { type: "lifecycle_rules"; id: string; attributes: LifecycleRule };
};

export async function listRules(bucket: string): Promise<LifecycleRule[]> {
  const res = await api.get<LifecycleListResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/lifecycle-rules`,
  );
  return res.data.map((d) => ({ ...d.attributes, id: d.id }));
}

export async function createRule(
  bucket: string,
  attributes: CreateRuleAttrs,
): Promise<LifecycleRule> {
  const res = await api.post<LifecycleSingleResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/lifecycle-rules`,
    { data: { type: "lifecycle_rules", attributes } },
  );
  return { ...res.data.attributes, id: res.data.id };
}

export async function deleteRule(bucket: string, id: string): Promise<void> {
  await api.delete<void>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/lifecycle-rules/${encodeURIComponent(id)}`,
  );
}
