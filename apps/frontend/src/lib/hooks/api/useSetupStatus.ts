import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { authKeys } from "@/lib/api/keys";

export type SetupStatus = { initialized: boolean };

export function useSetupStatus() {
  return useQuery({
    queryKey: authKeys.setupStatus(),
    queryFn: () => api.get<SetupStatus>("/api/v1/setup/status"),
  });
}
