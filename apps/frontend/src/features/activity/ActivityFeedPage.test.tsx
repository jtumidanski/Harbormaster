import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { ActivityFeedPage } from "./ActivityFeedPage";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function eventPayload(id: string, overrides: Record<string, unknown> = {}) {
  return {
    type: "audit_events",
    id,
    attributes: {
      id,
      occurred_at: "2024-06-01T12:00:00Z",
      actor: "alice",
      source_ip: "10.0.0.1",
      action: "bucket.create",
      target_type: "bucket",
      target_id: "photos",
      outcome: "success",
      error_message: null,
      payload_summary: {},
      ...overrides,
    },
  };
}

type FetchSpy = ReturnType<typeof vi.fn>;

function installAuditFetch(): FetchSpy {
  const spy = vi.fn((input: RequestInfo | URL) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    if (url.includes("/api/v1/audit-events")) {
      const params = new URL(url, "http://localhost").searchParams;
      const pageNum = Number(params.get("page[number]") ?? "1");
      const filterAction = params.get("filter[action]");
      const filterOutcome = params.get("filter[outcome]");
      const data =
        filterOutcome === "failure"
          ? [
              eventPayload("evt-failure", {
                action: "object.delete",
                outcome: "failure",
                error_message: "permission denied",
              }),
            ]
          : filterAction === "user.create"
            ? [eventPayload("evt-user", { action: "user.create", target_type: "user" })]
            : pageNum === 2
              ? [eventPayload("evt-p2", { id: "evt-p2", action: "object.put" })]
              : [eventPayload("evt-1")];
      return Promise.resolve(
        json({
          data,
          meta: { page: { number: pageNum, size: 50, total_pages: 3, total_records: 120 } },
        }),
      );
    }
    return Promise.resolve(json({ errors: [{ code: "not_found" }] }, 404));
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({
  children,
  qc,
  initialPath = "/activity",
}: PropsWithChildren<{ qc: QueryClient; initialPath?: string }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[initialPath]}>{children}</MemoryRouter>
    </QueryClientProvider>
  );
}

function lastUrl(spy: FetchSpy): string {
  const calls = spy.mock.calls
    .map(([url]) =>
      typeof url === "string" ? url : url instanceof URL ? url.toString() : (url as Request).url,
    )
    .filter((u) => u.includes("/api/v1/audit-events"));
  return calls[calls.length - 1] ?? "";
}

describe("ActivityFeedPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("renders filtered rows from URL search params", async () => {
    const spy = installAuditFetch();
    render(
      <Wrapper qc={makeQueryClient()} initialPath="/activity?outcome=failure">
        <ActivityFeedPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("object.delete")).toBeInTheDocument();
    });
    expect(screen.getByText(/permission denied/)).toBeInTheDocument();

    const url = lastUrl(spy);
    expect(url).toContain("filter%5Boutcome%5D=failure");
  });

  it("pagination Next changes page query param", async () => {
    const spy = installAuditFetch();
    const user = userEvent.setup();
    render(
      <Wrapper qc={makeQueryClient()}>
        <ActivityFeedPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("bucket.create")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /^next$/i }));

    await waitFor(() => {
      expect(screen.getByText("object.put")).toBeInTheDocument();
    });
    const url = lastUrl(spy);
    expect(url).toContain("page%5Bnumber%5D=2");
  });

  it("Reset clears all filters from URL", async () => {
    const spy = installAuditFetch();
    const user = userEvent.setup();
    render(
      <Wrapper qc={makeQueryClient()} initialPath="/activity?action=user.create&page=1">
        <ActivityFeedPage />
      </Wrapper>,
    );

    await waitFor(() => {
      expect(screen.getByText("user.create")).toBeInTheDocument();
    });

    const sidebar = screen.getByRole("form", { name: /filters/i });
    await user.click(within(sidebar).getByRole("button", { name: /reset/i }));

    await waitFor(() => {
      expect(screen.getByText("bucket.create")).toBeInTheDocument();
    });
    const url = lastUrl(spy);
    expect(url).not.toContain("filter%5Baction%5D");
  });
});
