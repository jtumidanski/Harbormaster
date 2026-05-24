import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { AuthProvider } from "@/context/AuthContext";
import { ThemeProvider } from "@/context/ThemeProvider";
import { AppRoutes } from "./routes";

type StubResponse = { match: (url: string) => boolean; response: () => Response };

function installFetch(stubs: StubResponse[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo) => {
      const url = typeof input === "string" ? input : input.url;
      for (const s of stubs) {
        if (s.match(url)) return Promise.resolve(s.response());
      }
      return Promise.resolve(
        new Response(JSON.stringify({ errors: [{ code: "not_found" }] }), {
          status: 404,
          headers: { "Content-Type": "application/vnd.api+json" },
        }),
      );
    }),
  );
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function Wrapper({ initialEntries, children }: PropsWithChildren<{ initialEntries: string[] }>) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
  return (
    <QueryClientProvider client={qc}>
      <ThemeProvider>
        <MemoryRouter initialEntries={initialEntries}>
          <AuthProvider>{children}</AuthProvider>
        </MemoryRouter>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

describe("AppRoutes", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders SetupWizard when initialized=false for any path", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/status"),
        response: () => json({ initialized: false }),
      },
      {
        match: (u) => u.includes("/api/v1/auth/me"),
        response: () =>
          new Response(JSON.stringify({ errors: [{ code: "unauthenticated" }] }), {
            status: 401,
            headers: { "Content-Type": "application/vnd.api+json" },
          }),
      },
    ]);
    render(
      <Wrapper initialEntries={["/some/path"]}>
        <AppRoutes />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByText(/setup wizard/i)).toBeInTheDocument();
    });
  });

  it("renders LoginPage when initialized=true and me=null", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/status"),
        response: () => json({ initialized: true }),
      },
      {
        match: (u) => u.includes("/api/v1/auth/me"),
        response: () =>
          new Response(JSON.stringify({ errors: [{ code: "unauthenticated" }] }), {
            status: 401,
            headers: { "Content-Type": "application/vnd.api+json" },
          }),
      },
    ]);
    render(
      <Wrapper initialEntries={["/"]}>
        <AppRoutes />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
    });
  });

  it("redirects / to /dashboard when authenticated and initialized", async () => {
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/status"),
        response: () => json({ initialized: true }),
      },
      {
        match: (u) => u.includes("/api/v1/auth/me"),
        response: () => json({ username: "alice", session_expires_at: "2030-01-01T00:00:00Z" }),
      },
      {
        match: (u) => u.includes("/api/v1/dashboard"),
        response: () =>
          json({
            server: { version: "0.0.0", deployment_mode: "single", uptime_seconds: 60 },
            totals: { buckets: 0, estimated_bytes: 0, objects: 0 },
            nodes: [],
            warnings: [],
            recent_activity: [],
            recent_failures: { window: "7d", count: 0, entries: [] },
          }),
      },
    ]);
    render(
      <Wrapper initialEntries={["/"]}>
        <AppRoutes />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /dashboard/i, level: 1 })).toBeInTheDocument();
    });
  });
});
