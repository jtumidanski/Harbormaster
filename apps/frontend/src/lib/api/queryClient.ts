import { MutationCache, QueryCache, QueryClient, hashKey } from "@tanstack/react-query";
import { toast } from "sonner";
import { authKeys } from "./keys";

// Session expiry surfaces as 401 with code "unauthenticated" (from the
// backend session middleware). Login and change-password rejections use
// 401 "invalid_credentials", so keying on the code keeps them out of the
// global logout path and lets their local onError handlers run instead.
export function isSessionExpiredError(err: unknown): boolean {
  const e = err as { status?: number; code?: string } | null | undefined;
  return e?.status === 401 && e?.code === "unauthenticated";
}

// Reset the cache to the signed-out state. Seed `me` as null FIRST so the
// always-mounted auth observer is notified and the route gate flips to
// /login — clear() would detach the observer from the query instance and
// the seed would go unseen. Then drop every other cached query so nothing
// stale survives into the next session. The setup-status query is kept:
// it is unauthenticated data that gates the whole router, and wiping it
// causes a pointless "Loading…" flash before the login screen.
export function resetToSignedOut(client: QueryClient): void {
  client.setQueryData(authKeys.me(), null);
  const keep = new Set([hashKey(authKeys.me()), hashKey(authKeys.setupStatus())]);
  client.removeQueries({ predicate: (query) => !keep.has(query.queryHash) });
}

// Route any API failure through this before showing a local error. Returns
// true when the error was a session expiry (caller should skip its own
// error handling — the route gate is flipping to /login).
export function handleSessionExpiry(client: QueryClient, err: unknown): boolean {
  if (!isSessionExpiredError(err)) return false;
  // Skip the reset when already signed out (or never signed in): concurrent
  // 401s and post-logout stragglers shouldn't re-toast.
  if (client.getQueryData(authKeys.me()) != null) {
    resetToSignedOut(client);
    toast.error("Your session has expired. Please sign in again.", {
      id: "session-expired",
    });
  }
  return true;
}

export function createQueryClient(): QueryClient {
  const client: QueryClient = new QueryClient({
    queryCache: new QueryCache({ onError: (err) => handleSessionExpiry(client, err) }),
    mutationCache: new MutationCache({ onError: (err) => handleSessionExpiry(client, err) }),
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
