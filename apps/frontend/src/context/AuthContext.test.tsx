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
