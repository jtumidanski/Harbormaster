import { api } from "@/lib/api/client";

// LifecycleRule mirrors the backend's discriminated union: managed rules
// carry the (kind, days, prefix) trio while unmanaged rules expose only
// a human-readable summary string. Both flags share `managed`.
export type LifecycleRule = {
  id: string;
  managed: boolean;
  kind?: string;
  days?: number;
  prefix?: string;
  summary?: string;
};

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
  days: number,
  prefix?: string,
): Promise<LifecycleRule> {
  // The backend expects a JSON:API single-resource envelope with
  // attributes {kind, days, prefix} and only kind="expiration" is
  // accepted in v1.
  const res = await api.post<LifecycleSingleResponse>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/lifecycle-rules`,
    {
      data: {
        type: "lifecycle_rules",
        attributes: {
          kind: "expiration",
          days,
          prefix: prefix ?? "",
        },
      },
    },
  );
  return { ...res.data.attributes, id: res.data.id };
}

export async function deleteRule(bucket: string, id: string): Promise<void> {
  await api.delete<void>(
    `/api/v1/buckets/${encodeURIComponent(bucket)}/lifecycle-rules/${encodeURIComponent(id)}`,
  );
}
