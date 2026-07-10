import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { authKeys } from "./keys";

export function createQueryClient(): QueryClient {
  // Session expiry surfaces as 401 with code "unauthenticated" (from the
  // backend session middleware). Login and change-password rejections use
  // 401 "invalid_credentials", so keying on the code keeps them out of the
  // global logout path and lets their local onError handlers run instead.
  const onAuthError = (err: unknown) => {
    const e = err as { status?: number; code?: string };
    if (e.status !== 401 || e.code !== "unauthenticated") return;
    // Already signed out (or never signed in) — the route gate is on /login.
    if (client.getQueryData(authKeys.me()) == null) return;
    // Seed `me` as null FIRST so the always-mounted auth observer is notified
    // and the route gate flips to /login (clear() would detach the observer
    // from the query instance, and the seed would go unseen). Then drop every
    // other cached query so nothing stale survives into the next session.
    const meKey = JSON.stringify(authKeys.me());
    client.setQueryData(authKeys.me(), null);
    client.removeQueries({
      predicate: (query) => JSON.stringify(query.queryKey) !== meKey,
    });
    toast.error("Your session has expired. Please sign in again.", {
      id: "session-expired",
    });
  };

  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError: onAuthError }),
    mutationCache: new MutationCache({ onError: onAuthError }),
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        retry: (failureCount, err) => {
          const status = (err as { status?: number }).status;
          if (status && status >= 400 && status < 500) return false;
          return failureCount < 2;
        },
      },
    },
  });
  return client;
}
