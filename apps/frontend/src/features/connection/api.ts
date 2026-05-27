import { api } from "@/lib/api/client";

export type ConnectionDetail = {
  endpoint_url: string;
  tls_skip_verify: boolean;
  access_key_masked: string;
  secret_key_present: boolean;
  custom_ca_pem_present: boolean;
};

export type ConnectionPayload = {
  endpoint_url: string;
  access_key: string;
  secret_key: string;
  tls_skip_verify: boolean;
  custom_ca_pem: string | null;
};

export type ConnectionCheck = "ok" | null | { failed: string };

export type ConnectionTestResult = {
  tcp_connect: ConnectionCheck;
  list_buckets: ConnectionCheck;
  admin_ping: ConnectionCheck;
  minio_version: string | null;
};

export function fetchConnection() {
  return api.get<ConnectionDetail>("/api/v1/connection");
}

export function updateConnection(p: ConnectionPayload) {
  return api.put<void>("/api/v1/connection", p);
}

export function testConnection(p: ConnectionPayload) {
  return api.post<ConnectionTestResult>("/api/v1/connection/test", p);
}
