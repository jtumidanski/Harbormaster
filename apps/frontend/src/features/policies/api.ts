import { api } from "@/lib/api/client";
import type { PolicyTemplate, PolicyTemplateCollectionResponse } from "./types";

export async function listPolicyTemplates(): Promise<PolicyTemplate[]> {
  const res = await api.get<PolicyTemplateCollectionResponse>("/api/v1/policy-templates");
  return res.data.map((d) => d.attributes);
}
