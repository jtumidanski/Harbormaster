import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";
import { AuthProvider, useAuth } from "./AuthContext";

function stubFetch(response: Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: RequestInfo, _init?: RequestInit) => Promise.resolve(response)),
  );
}

function Wrapper({ children }: PropsWithChildren) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
  return (
    <QueryClientProvider client={qc}>
      <AuthProvider>{children}</AuthProvider>
    </QueryClientProvider>
  );
}

function Probe() {
  const { me, isLoading } = useAuth();
  if (isLoading) return <div>loading</div>;
  return <div data-testid="me">{me ? me.username : "anon"}</div>;
}

describe("AuthContext", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  afterEach(() => {
    cleanup();
  });

  it("yields me=null when /auth/me returns 401", async () => {
    stubFetch(
      new Response(JSON.stringify({ errors: [{ code: "unauthenticated" }] }), {
        status: 401,
        headers: { "Content-Type": "application/vnd.api+json" },
      }),
    );
    render(
      <Wrapper>
        <Probe />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByTestId("me").textContent).toBe("anon");
    });
  });

  it("logout flips me to anon synchronously and drops cached data", async () => {
    // /auth/me keeps answering 200 — the signed-out state must come from
    // the cache reset itself, not from a lucky refetch racing a dead cookie.
    const fetchSpy = vi.fn((input: RequestInfo, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.url;
      if (url.includes("/api/v1/auth/logout") && init?.method === "POST") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({ username: "alice", session_expires_at: "2030-01-01T00:00:00Z" }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    });
    vi.stubGlobal("fetch", fetchSpy);

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
    });
    qc.setQueryData(["buckets", "list"], [{ name: "b1" }]);

    function LogoutProbe() {
      const { me, isLoading, logout } = useAuth();
      if (isLoading) return <div>loading</div>;
      return (
        <div>
          <div data-testid="me">{me ? me.username : "anon"}</div>
          <button onClick={() => void logout()}>do-logout</button>
        </div>
      );
    }

    render(
      <QueryClientProvider client={qc}>
        <AuthProvider>
          <LogoutProbe />
        </AuthProvider>
      </QueryClientProvider>,
    );
    await waitFor(() => {
      expect(screen.getByTestId("me").textContent).toBe("alice");
    });

    screen.getByRole("button", { name: "do-logout" }).click();

    await waitFor(() => {
      expect(screen.getByTestId("me").textContent).toBe("anon");
    });
    expect(qc.getQueryData(["buckets", "list"])).toBeUndefined();
    expect(
      fetchSpy.mock.calls.some(
        ([input, init]) =>
          (typeof input === "string" ? input : input.url).includes("/api/v1/auth/logout") &&
          init?.method === "POST",
      ),
    ).toBe(true);
  });

  it("yields me.username when /auth/me returns 200", async () => {
    stubFetch(
      new Response(
        JSON.stringify({
          username: "alice",
          session_expires_at: "2030-01-01T00:00:00Z",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    render(
      <Wrapper>
        <Probe />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByTestId("me").textContent).toBe("alice");
    });
  });
});
