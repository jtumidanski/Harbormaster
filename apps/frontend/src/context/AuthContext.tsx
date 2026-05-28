import { createContext, useContext, useMemo, type PropsWithChildren } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { authKeys } from "@/lib/api/keys";

export type Me = { username: string; session_expires_at: string };

type Ctx = {
  me: Me | null;
  isLoading: boolean;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
};

const AuthCtx = createContext<Ctx | null>(null);

export function AuthProvider({ children }: PropsWithChildren) {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery<Me | null>({
    queryKey: authKeys.me(),
    queryFn: async () => {
      try {
        return await api.get<Me>("/api/v1/auth/me");
      } catch (e) {
        if ((e as { status?: number }).status === 401) return null;
        throw e;
      }
    },
  });

  const value = useMemo<Ctx>(
    () => ({
      me: data ?? null,
      isLoading,
      refresh: async () => {
        await qc.invalidateQueries({ queryKey: authKeys.me() });
      },
      logout: async () => {
        try {
          await api.post("/api/v1/auth/logout");
        } finally {
          // Drop every cached query, then explicitly seed `me` as null so the
          // auth gate flips to the unauthenticated routes synchronously. Relying
          // on clear()'s refetch left a window where the stale `me` was still
          // truthy, so /login fell through to the authenticated "Not found".
          qc.clear();
          qc.setQueryData(authKeys.me(), null);
        }
      },
    }),
    [data, isLoading, qc],
  );

  return <AuthCtx.Provider value={value}>{children}</AuthCtx.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthCtx);
  if (!ctx) throw new Error("useAuth outside AuthProvider");
  return ctx;
}
