import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { authKeys } from "@/lib/api/keys";

export type Me = { username: string; session_expires_at: string };

export function useMe() {
  return useQuery({
    queryKey: authKeys.me(),
    queryFn: () => api.get<Me>("/api/v1/auth/me"),
  });
}
