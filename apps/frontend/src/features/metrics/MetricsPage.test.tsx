import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { MetricsPage } from "./MetricsPage";
import type { MetricsView } from "./types";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function pausedView(): MetricsView {
  return {
    window: "24h",
    step_seconds: 60,
    collected: false,
    series: {},
  };
}

function collectedView(): MetricsView {
  return {
    window: "24h",
    step_seconds: 60,
    collected: true,
    series: {
      minio_s3_requests_total: [
        { t: "2026-01-01T00:00:00Z", v: 1.2 },
        { t: "2026-01-01T00:01:00Z", v: 1.5 },
        { t: "2026-01-01T00:02:00Z", v: 0.8 },
      ],
      minio_s3_requests_4xx_errors_total: [
        { t: "2026-01-01T00:00:00Z", v: 0.1 },
        { t: "2026-01-01T00:01:00Z", v: 0.0 },
      ],
      minio_s3_requests_5xx_errors_total: [],
      minio_s3_traffic_received_bytes: [
        { t: "2026-01-01T00:00:00Z", v: 1024.0 },
        { t: "2026-01-01T00:01:00Z", v: 2048.0 },
      ],
      minio_s3_traffic_sent_bytes: [
        { t: "2026-01-01T00:00:00Z", v: 512.0 },
        { t: "2026-01-01T00:01:00Z", v: 768.0 },
      ],
      minio_cluster_capacity_usable_total_bytes: [{ t: "2026-01-01T00:00:00Z", v: 1_000_000_000 }],
      minio_cluster_capacity_usable_free_bytes: [{ t: "2026-01-01T00:00:00Z", v: 600_000_000 }],
      minio_cluster_drive_online_total: [{ t: "2026-01-01T00:00:00Z", v: 4 }],
      minio_cluster_drive_offline_total: [{ t: "2026-01-01T00:00:00Z", v: 0 }],
    },
  };
}

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/metrics"]}>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

describe("MetricsPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    window.localStorage.clear();
  });

  afterEach(() => {
    cleanup();
  });

  it("shows the paused banner and no charts when collected is false", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(json(pausedView()))),
    );

    render(
      <Wrapper qc={makeQueryClient()}>
        <MetricsPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });

    // Banner text should mention collection paused
    expect(screen.getByRole("alert")).toHaveTextContent(/collection paused/i);

    // No chart containers should be visible
    expect(screen.queryByTestId("metrics-chart-requests")).not.toBeInTheDocument();
    expect(screen.queryByTestId("metrics-chart-errors")).not.toBeInTheDocument();
    expect(screen.queryByTestId("metrics-chart-throughput")).not.toBeInTheDocument();
    expect(screen.queryByTestId("metrics-chart-capacity")).not.toBeInTheDocument();
  });

  it("renders chart containers when collected is true", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(json(collectedView()))),
    );

    render(
      <Wrapper qc={makeQueryClient()}>
        <MetricsPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("metrics-chart-requests")).toBeInTheDocument();
    });

    expect(screen.getByTestId("metrics-chart-errors")).toBeInTheDocument();
    expect(screen.getByTestId("metrics-chart-throughput")).toBeInTheDocument();
    expect(screen.getByTestId("metrics-chart-capacity")).toBeInTheDocument();

    // No paused banner
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
