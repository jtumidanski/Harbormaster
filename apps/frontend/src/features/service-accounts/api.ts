import { api } from "@/lib/api/client";
import type { TemplateRef } from "@/features/users/types";

export type ServiceAccount = {
  access_key: string;
  parent_user: string;
  name: string;
  description: string;
  attached_template: TemplateRef | null;
};

export type ServiceAccountResource = {
  type: "service_accounts";
  id: string;
  attributes: ServiceAccount;
};

export type ServiceAccountCollectionResponse = {
  data: ServiceAccountResource[];
};

export type CreateServiceAccountAttrs = ServiceAccount & { secret_key: string };

export type CreateServiceAccountResponse = {
  data: {
    type: "service_accounts";
    id: string;
    attributes: CreateServiceAccountAttrs;
  };
};

export async function listServiceAccounts(accessKey: string): Promise<ServiceAccount[]> {
  const res = await api.get<ServiceAccountCollectionResponse>(
    `/api/v1/users/${encodeURIComponent(accessKey)}/service-accounts`,
  );
  return res.data.map((d) => d.attributes);
}

export type CreateServiceAccountInput = {
  name: string;
  description: string;
  template_override?: TemplateRef | null;
};

export async function createServiceAccount(
  accessKey: string,
  input: CreateServiceAccountInput,
): Promise<CreateServiceAccountAttrs> {
  const res = await api.post<CreateServiceAccountResponse>(
    `/api/v1/users/${encodeURIComponent(accessKey)}/service-accounts`,
    {
      data: {
        type: "service_accounts",
        attributes: input,
      },
    },
  );
  return res.data.attributes;
}

export async function revokeServiceAccount(serviceAccountKey: string): Promise<void> {
  await api.delete<void>(`/api/v1/service-accounts/${encodeURIComponent(serviceAccountKey)}`);
}
