import { api } from "@/lib/api/client";

export type McAlias = {
  name: string;
  endpoint: string;
  access_key: string;
  tls_skip_verify: boolean;
};

export function fetchMcAliases() {
  return api.get<{ aliases: McAlias[] }>("/api/v1/setup/mc-aliases");
}

export type SetupExplicitMinIO = {
  endpoint_url: string;
  access_key: string;
  secret_key: string;
  tls_skip_verify: boolean;
  custom_ca_pem: string | null;
};

export type SetupAliasMinIO = {
  from_mc_alias: string;
  tls_skip_verify: boolean;
  custom_ca_pem: string | null;
};

export type SetupPayload = {
  admin: { username: string; password: string };
  minio: SetupExplicitMinIO | SetupAliasMinIO;
};

export function submitSetup(payload: SetupPayload) {
  return api.post<{ initialized: true }>("/api/v1/setup", payload);
}
