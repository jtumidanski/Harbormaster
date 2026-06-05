import { api } from "@/lib/api/client";
import type { Policy, PolicyDetail, PolicyCollectionResponse, PolicySingleResponse } from "./types";

export async function listPolicies(): Promise<Policy[]> {
  const res = await api.get<PolicyCollectionResponse>("/api/v1/policies");
  return res.data.map((d) => d.attributes);
}

export async function getPolicy(name: string): Promise<PolicyDetail> {
  const res = await api.get<PolicySingleResponse>(`/api/v1/policies/${encodeURIComponent(name)}`);
  return res.data.attributes;
}

export async function createPolicy(name: string, document: unknown): Promise<void> {
  await api.post("/api/v1/policies", {
    data: {
      type: "policies",
      attributes: { name, document },
    },
  });
}

export async function updatePolicy(name: string, document: unknown): Promise<void> {
  await api.put(`/api/v1/policies/${encodeURIComponent(name)}`, {
    data: {
      type: "policies",
      attributes: { document },
    },
  });
}

export async function deletePolicy(name: string): Promise<void> {
  await api.delete(`/api/v1/policies/${encodeURIComponent(name)}`);
}
