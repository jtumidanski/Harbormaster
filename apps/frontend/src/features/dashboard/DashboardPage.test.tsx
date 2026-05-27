import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { DashboardPage } from "./DashboardPage";
import type { DashboardView, DashboardWindow } from "./types";

function viewFor(window: DashboardWindow): DashboardView {
  return {
    server: { version: "1.2.3", deployment_mode: "single", uptime_seconds: 60 * 60 * 26 + 13 * 60 },
    totals: { buckets: 5, estimated_bytes: 1024 * 1024 * 1024, objects: 12345 },
    nodes: [
      {
        endpoint: "node-1:9000",
        state: "online",
        drives: { total: 4, healthy: 4, unhealthy: 0 },
      },
      {
        endpoint: "node-2:9000",
        state: "offline",
        drives: { total: 4, healthy: 3, unhealthy: 1 },
      },
    ],
    warnings: ["Disk usage above 80%"],
    recent_activity: [
      {
        id: "e1",
        occurred_at: new Date(Date.now() - 60_000).toISOString(),
        action: "bucket.create",
        target_type: "bucket",
        target_id: "photos",
        outcome: "success",
      },
    ],
    recent_failures: {
      window,
      count: 3,
      entries: [
        {
          id: "f1",
          occurred_at: new Date().toISOString(),
          action: "object.delete",
          target_type: "object",
          target_id: "photos/x.jpg",
          outcome: "failure",
          source_ip: "10.0.0.5",
          error_message: "permission denied",
        },
      ],
    },
  };
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type FetchSpy = ReturnType<typeof vi.fn>;

function installDashboardFetch(): FetchSpy {
  const spy = vi.fn((input: RequestInfo | URL) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    const m = url.match(/failures_window=(24h|7d|30d)/);
    if (url.includes("/api/v1/dashboard") && m) {
      const win = m[1] as DashboardWindow;
      return Promise.resolve(json(viewFor(win)));
    }
    return Promise.resolve(
      new Response(JSON.stringify({ errors: [{ code: "not_found" }] }), {
        status: 404,
        headers: { "Content-Type": "application/json" },
      }),
    );
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/dashboard"]}>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

describe("DashboardPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    window.localStorage.clear();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders all sections from the stubbed response", async () => {
    installDashboardFetch();
    render(
      <Wrapper qc={makeQueryClient()}>
        <DashboardPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("1.2.3")).toBeInTheDocument();
    });
    expect(screen.getByRole("heading", { name: /dashboard/i, level: 1 })).toBeInTheDocument();
    expect(screen.getByText("single")).toBeInTheDocument();
    expect(screen.getByText("1d 2h 13m")).toBeInTheDocument();

    // Totals
    expect(screen.getByText("Buckets")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("12,345")).toBeInTheDocument();

    // Warnings
    expect(screen.getByText(/disk usage above 80%/i)).toBeInTheDocument();

    // Nodes
    expect(screen.getByText("node-1:9000")).toBeInTheDocument();
    expect(screen.getByText("node-2:9000")).toBeInTheDocument();
    expect(screen.getByText(/4\/4 healthy/)).toBeInTheDocument();

    // Activity + failures widget
    expect(screen.getByText("Recent activity")).toBeInTheDocument();
    expect(screen.getByText("bucket.create")).toBeInTheDocument();
    expect(screen.getByText("Recent failures")).toBeInTheDocument();
    expect(screen.getByText(/3 failures in 7d/)).toBeInTheDocument();
    expect(screen.getByText(/permission denied/)).toBeInTheDocument();
  });

  it("reads localStorage-persisted failures window value", async () => {
    window.localStorage.setItem("harbormaster:dashboard:failuresWindow", "24h");
    const spy = installDashboardFetch();
    render(
      <Wrapper qc={makeQueryClient()}>
        <DashboardPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText(/3 failures in 24h/)).toBeInTheDocument();
    });

    const initialCall = spy.mock.calls.find(([url]) => {
      const u =
        typeof url === "string" ? url : url instanceof URL ? url.toString() : (url as Request).url;
      return u.includes("/api/v1/dashboard");
    });
    expect(initialCall).toBeDefined();
    const initialUrl =
      typeof initialCall![0] === "string" ? initialCall![0] : (initialCall![0] as URL).toString();
    expect(initialUrl).toContain("failures_window=24h");
  });

  it("changing the widget window triggers a new fetch with the new param", async () => {
    const spy = installDashboardFetch();
    const user = userEvent.setup();
    render(
      <Wrapper qc={makeQueryClient()}>
        <DashboardPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText(/3 failures in 7d/)).toBeInTheDocument();
    });

    // Open the window select and pick 30 days.
    const trigger = screen.getByRole("combobox", { name: /failures window/i });
    await user.click(trigger);
    const option = await screen.findByRole("option", { name: /last 30 days/i });
    await user.click(option);

    await waitFor(() => {
      expect(screen.getByText(/3 failures in 30d/)).toBeInTheDocument();
    });

    const calls = spy.mock.calls
      .map(([url]) =>
        typeof url === "string" ? url : url instanceof URL ? url.toString() : (url as Request).url,
      )
      .filter((u) => u.includes("/api/v1/dashboard"));

    expect(calls.some((u) => u.includes("failures_window=30d"))).toBe(true);
    expect(window.localStorage.getItem("harbormaster:dashboard:failuresWindow")).toBe("30d");
  });
});
