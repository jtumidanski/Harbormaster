import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { AuthProvider } from "@/context/AuthContext";
import { ThemeProvider } from "@/context/ThemeProvider";
import { createQueryClient } from "@/lib/api/queryClient";
import { AppRoutes } from "./routes";

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

function installFetch(stubs: StubResponse[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.url;
      for (const s of stubs) {
        if (s.match(url, init)) return Promise.resolve(s.response());
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

function Wrapper({
  initialEntries,
  queryClient,
  children,
}: PropsWithChildren<{ initialEntries: string[]; queryClient?: QueryClient }>) {
  const qc =
    queryClient ??
    new QueryClient({
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

  it("returns to the login screen when a page query fails with 401 unauthenticated", async () => {
    // Simulates an expired session: /auth/me succeeds once (cached at app
    // start), then the session dies server-side and every endpoint 401s.
    let meCalls = 0;
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/status"),
        response: () => json({ initialized: true }),
      },
      {
        match: (u) => u.includes("/api/v1/auth/me"),
        response: () => {
          meCalls += 1;
          if (meCalls === 1) {
            return json({ username: "alice", session_expires_at: "2030-01-01T00:00:00Z" });
          }
          return new Response(
            JSON.stringify({ errors: [{ code: "unauthenticated", detail: "expired" }] }),
            { status: 401, headers: { "Content-Type": "application/vnd.api+json" } },
          );
        },
      },
      {
        match: (u) => u.includes("/api/v1/dashboard"),
        response: () =>
          new Response(
            JSON.stringify({
              errors: [{ code: "unauthenticated", detail: "Authentication required." }],
            }),
            { status: 401, headers: { "Content-Type": "application/vnd.api+json" } },
          ),
      },
    ]);
    render(
      <Wrapper initialEntries={["/dashboard"]} queryClient={createQueryClient()}>
        <AppRoutes />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
    });
  });

  it("returns to the originally requested page after signing in", async () => {
    let loggedIn = false;
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/status"),
        response: () => json({ initialized: true }),
      },
      {
        match: (u, init) => u.includes("/api/v1/auth/login") && init?.method === "POST",
        response: () => {
          loggedIn = true;
          return new Response(null, { status: 204 });
        },
      },
      {
        match: (u) => u.includes("/api/v1/auth/me"),
        response: () =>
          loggedIn
            ? json({ username: "alice", session_expires_at: "2030-01-01T00:00:00Z" })
            : new Response(JSON.stringify({ errors: [{ code: "unauthenticated" }] }), {
                status: 401,
                headers: { "Content-Type": "application/vnd.api+json" },
              }),
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
    const user = userEvent.setup();
    render(
      <Wrapper initialEntries={["/dashboard"]} queryClient={createQueryClient()}>
        <AppRoutes />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
    });
    await user.type(screen.getByLabelText(/username/i), "alice");
    await user.type(screen.getByLabelText(/password/i), "supersecretpwd");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    // Lands back on /dashboard, not the default /buckets.
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /dashboard/i, level: 1 })).toBeInTheDocument();
    });
  });

  it("lands on /buckets after signing in from a direct /login visit", async () => {
    let loggedIn = false;
    installFetch([
      {
        match: (u) => u.includes("/api/v1/setup/status"),
        response: () => json({ initialized: true }),
      },
      {
        match: (u, init) => u.includes("/api/v1/auth/login") && init?.method === "POST",
        response: () => {
          loggedIn = true;
          return new Response(null, { status: 204 });
        },
      },
      {
        match: (u) => u.includes("/api/v1/auth/me"),
        response: () =>
          loggedIn
            ? json({ username: "alice", session_expires_at: "2030-01-01T00:00:00Z" })
            : new Response(JSON.stringify({ errors: [{ code: "unauthenticated" }] }), {
                status: 401,
                headers: { "Content-Type": "application/vnd.api+json" },
              }),
      },
    ]);
    const user = userEvent.setup();
    render(
      <Wrapper initialEntries={["/login"]} queryClient={createQueryClient()}>
        <AppRoutes />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /sign in/i })).toBeInTheDocument();
    });
    await user.type(screen.getByLabelText(/username/i), "alice");
    await user.type(screen.getByLabelText(/password/i), "supersecretpwd");
    await user.click(screen.getByRole("button", { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /buckets/i, level: 1 })).toBeInTheDocument();
    });
  });
});
