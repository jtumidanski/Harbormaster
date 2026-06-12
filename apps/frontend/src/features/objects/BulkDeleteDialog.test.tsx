import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ComponentProps } from "react";
import { Toaster } from "sonner";
import { BulkDeleteDialog } from "./BulkDeleteDialog";

function renderDialog(props: Partial<ComponentProps<typeof BulkDeleteDialog>> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onDeleted = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <BulkDeleteDialog
        open
        onOpenChange={onOpenChange}
        bucket="b"
        keys={[]}
        prefixes={["photos/"]}
        onDeleted={onDeleted}
        {...props}
      />
      <Toaster />
    </QueryClientProvider>,
  );
  return { onDeleted, onOpenChange };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("BulkDeleteDialog", () => {
  it("shows the dry-run count and enables Delete once loaded", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(jsonResponse({ object_count: 42, truncated: false }))),
    );
    renderDialog();
    await waitFor(() => expect(screen.getByText(/42/)).toBeInTheDocument());
    const del = screen.getByRole("button", { name: /^delete$/i });
    expect(del).toBeEnabled();
  });

  it("renders 10,000+ when truncated", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.resolve(jsonResponse({ object_count: 10000, truncated: true }))),
    );
    renderDialog();
    await waitFor(() => expect(screen.getByText(/10,000\+/)).toBeInTheDocument());
  });

  it("reports a partial-failure toast and calls onDeleted", async () => {
    const fetchSpy = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      const isDelete =
        typeof init?.body === "string" && init.body.includes('"dry_run":false');
      if (isDelete) {
        return Promise.resolve(
          jsonResponse({ deleted_count: 2, failures: [{ key: "photos/x", error: "boom" }] }),
        );
      }
      return Promise.resolve(jsonResponse({ object_count: 3, truncated: false }));
    });
    vi.stubGlobal("fetch", fetchSpy);
    const { onDeleted } = renderDialog();

    await waitFor(() => expect(screen.getByText(/3/)).toBeInTheDocument());
    await userEvent.click(screen.getByRole("button", { name: /^delete$/i }));

    await waitFor(() => expect(onDeleted).toHaveBeenCalled());
    expect(await screen.findByText(/1 failed/i)).toBeInTheDocument();
  });
});
