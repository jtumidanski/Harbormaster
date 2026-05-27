import { api } from "@/lib/api/client";

export function login(body: { username: string; password: string }) {
  return api.post<void>("/api/v1/auth/login", body);
}

export function changePassword(body: { current_password: string; new_password: string }) {
  return api.post<void>("/api/v1/auth/password", body);
}
