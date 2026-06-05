import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { AppError } from "@/lib/api/errors";
import { PoliciesPage } from "./PoliciesPage";
import * as policiesApi from "./policiesApi";
import type { Policy } from "./types";

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
}

function Wrapper({ children, qc }: PropsWithChildren<{ qc: QueryClient }>) {
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        {children}
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

const customPolicy: Policy = {
  name: "my-custom-policy",
  origin: "custom",
  editable: true,
  statement_summary: "Allow s3:GetObject on *",
};

const builtinPolicy: Policy = {
  name: "readwrite",
  origin: "minio-builtin",
  editable: false,
  statement_summary: "MinIO built-in ReadWrite",
};

describe("PoliciesPage", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  describe("editable-gating", () => {
    it("shows Edit/Delete buttons only for editable (custom) policies, not for minio-builtin", async () => {
      vi.spyOn(policiesApi, "listPolicies").mockResolvedValue([customPolicy, builtinPolicy]);
      const qc = makeQueryClient();
      render(
        <Wrapper qc={qc}>
          <PoliciesPage />
        </Wrapper>,
      );

      // Wait for the table to populate
      await waitFor(() => {
        expect(screen.getByText("my-custom-policy")).toBeInTheDocument();
      });
      expect(screen.getByText("readwrite")).toBeInTheDocument();

      // Custom policy row: Edit and Delete buttons must be present
      expect(screen.getByRole("button", { name: /^edit$/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^delete$/i })).toBeInTheDocument();

      // Built-in policy row: no Edit/Delete buttons rendered at all
      const editButtons = screen.getAllByRole("button", { name: /^edit$/i });
      const deleteButtons = screen.getAllByRole("button", { name: /^delete$/i });
      // Only one of each — the custom policy row
      expect(editButtons).toHaveLength(1);
      expect(deleteButtons).toHaveLength(1);
    });
  });

  describe("delete — policy_in_use with details flowing through", () => {
    it("shows alice and team-x in the confirm dialog when deletePolicy rejects with policy_in_use", async () => {
      vi.spyOn(policiesApi, "listPolicies").mockResolvedValue([customPolicy]);
      vi.spyOn(policiesApi, "deletePolicy").mockRejectedValue(
        new AppError({
          status: 409,
          code: "policy_in_use",
          message: "Policy is still in use",
          details: {
            attached_to: {
              users: ["alice"],
              groups: ["team-x"],
            },
          },
        }),
      );

      const user = userEvent.setup();
      const qc = makeQueryClient();
      render(
        <Wrapper qc={qc}>
          <PoliciesPage />
        </Wrapper>,
      );

      // Wait for policy row
      await waitFor(() => {
        expect(screen.getByText("my-custom-policy")).toBeInTheDocument();
      });

      // Open the delete confirm dialog
      await user.click(screen.getByRole("button", { name: /^delete$/i }));

      // Dialog should appear
      await waitFor(() => {
        expect(screen.getByRole("dialog")).toBeInTheDocument();
      });

      // Click the destructive Delete button inside the dialog
      const dialogDeleteButton = screen.getByRole("button", { name: /^delete$/i });
      await user.click(dialogDeleteButton);

      // After deletePolicy rejects with policy_in_use, the in-use panel with alice and team-x must appear
      await waitFor(() => {
        expect(screen.getByText("alice")).toBeInTheDocument();
      });
      expect(screen.getByText("team-x")).toBeInTheDocument();
      // Confirm the descriptive error text is visible
      expect(screen.getByText(/policy is still in use/i)).toBeInTheDocument();
    });
  });
});
