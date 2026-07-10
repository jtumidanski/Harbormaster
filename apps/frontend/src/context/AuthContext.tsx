import { createContext, useContext, useMemo, type PropsWithChildren } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import { authKeys } from "@/lib/api/keys";
import { resetToSignedOut } from "@/lib/api/queryClient";

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
          // Shared with the global session-expiry handler: seeds `me` null
          // first (flipping the auth gate synchronously), then drops the
          // other cached queries. The old clear()-then-seed order detached
          // the mounted observer, leaving the flip to a refetch race
          // against the dying session cookie.
          resetToSignedOut(qc);
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
