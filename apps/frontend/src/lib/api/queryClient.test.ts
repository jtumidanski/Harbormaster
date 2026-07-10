import { describe, it, expect, beforeEach, vi } from "vitest";
import { MutationObserver, type QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { AppError } from "./errors";
import { authKeys } from "./keys";
import { createQueryClient } from "./queryClient";

vi.mock("sonner", () => ({
  toast: { error: vi.fn() },
}));

const alice = { username: "alice", session_expires_at: "2030-01-01T00:00:00Z" };

function sessionExpired(): AppError {
  return new AppError({
    status: 401,
    code: "unauthenticated",
    message: "Authentication required.",
  });
}

async function failQuery(qc: QueryClient, err: AppError): Promise<void> {
  await qc
    .fetchQuery({
      queryKey: ["buckets", "list"],
      queryFn: () => Promise.reject(err),
      retry: false,
    })
    .catch(() => {});
}

describe("createQueryClient session-expiry handling", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("clears the cache and nulls auth.me when a query fails with 401 unauthenticated", async () => {
    const qc = createQueryClient();
    qc.setQueryData(authKeys.me(), alice);
    qc.setQueryData(["policies", "list"], [{ id: "readonly" }]);

    await failQuery(qc, sessionExpired());

    expect(qc.getQueryData(authKeys.me())).toBeNull();
    expect(qc.getQueryData(["policies", "list"])).toBeUndefined();
    expect(toast.error).toHaveBeenCalledWith(
      expect.stringMatching(/session.*expired/i),
      expect.anything(),
    );
  });

  it("preserves the setup-status query so the route gate doesn't flash loading", async () => {
    const qc = createQueryClient();
    qc.setQueryData(authKeys.me(), alice);
    qc.setQueryData(authKeys.setupStatus(), { initialized: true });

    await failQuery(qc, sessionExpired());

    expect(qc.getQueryData(authKeys.me())).toBeNull();
    expect(qc.getQueryData(authKeys.setupStatus())).toEqual({ initialized: true });
  });

  it("nulls auth.me when a mutation fails with 401 unauthenticated", async () => {
    const qc = createQueryClient();
    qc.setQueryData(authKeys.me(), alice);

    const observer = new MutationObserver(qc, {
      mutationFn: () => Promise.reject(sessionExpired()),
    });
    await observer.mutate().catch(() => {});

    expect(qc.getQueryData(authKeys.me())).toBeNull();
  });

  it("ignores 401 invalid_credentials (login / change-password rejections)", async () => {
    const qc = createQueryClient();
    qc.setQueryData(authKeys.me(), alice);

    await failQuery(
      qc,
      new AppError({
        status: 401,
        code: "invalid_credentials",
        message: "Current password incorrect",
      }),
    );

    expect(qc.getQueryData(authKeys.me())).toEqual(alice);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("ignores non-401 errors", async () => {
    const qc = createQueryClient();
    qc.setQueryData(authKeys.me(), alice);

    await failQuery(qc, new AppError({ status: 500, code: "internal", message: "boom" }));

    expect(qc.getQueryData(authKeys.me())).toEqual(alice);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("does nothing when already signed out", async () => {
    const qc = createQueryClient();
    qc.setQueryData(authKeys.me(), null);

    await failQuery(qc, sessionExpired());

    expect(qc.getQueryData(authKeys.me())).toBeNull();
    expect(toast.error).not.toHaveBeenCalled();
  });
});
