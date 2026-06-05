import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { CreateRuleDialog } from "./CreateRuleDialog";
import * as api from "./api";

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

describe("CreateRuleDialog", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("defaults to expiration kind and submits with {kind, days, prefix}", async () => {
    const spy = vi.spyOn(api, "createRule").mockResolvedValue({
      id: "rule-1",
      managed: true,
      kind: "expiration",
      days: 30,
      prefix: "",
    });
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateRuleDialog open={true} onOpenChange={() => {}} bucket="photos" />
      </Wrapper>,
    );

    // The days field should be visible (expiration is default kind)
    const daysInput = screen.getByLabelText(/^days$/i);
    expect(daysInput).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /add rule/i }));

    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith("photos", {
        kind: "expiration",
        days: 30,
        prefix: "",
      });
    });
  });

  it("selecting Noncurrent versions reveals noncurrent_days field and submits correct payload", async () => {
    const spy = vi.spyOn(api, "createRule").mockResolvedValue({
      id: "rule-2",
      managed: true,
      kind: "noncurrent-expiration",
      noncurrent_days: 7,
      newer_noncurrent_versions: 3,
      prefix: "archive/",
    });
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateRuleDialog open={true} onOpenChange={() => {}} bucket="photos" />
      </Wrapper>,
    );

    // Open the kind select and pick Noncurrent versions
    const kindTrigger = screen.getByRole("combobox", { name: /rule kind/i });
    await user.click(kindTrigger);
    const option = await screen.findByRole("option", { name: /noncurrent versions/i });
    await user.click(option);

    // The noncurrent_days field should now be visible
    const noncurrentDaysInput = await screen.findByLabelText(/noncurrent days/i);
    expect(noncurrentDaysInput).toBeInTheDocument();

    // The newer_noncurrent_versions field should also be visible
    const newerVersionsInput = screen.getByLabelText(/newer noncurrent versions/i);
    expect(newerVersionsInput).toBeInTheDocument();

    // days field from expiration should NOT be visible
    expect(screen.queryByLabelText(/^days$/i)).not.toBeInTheDocument();

    // Fill in values
    await user.clear(noncurrentDaysInput);
    await user.type(noncurrentDaysInput, "7");
    await user.clear(newerVersionsInput);
    await user.type(newerVersionsInput, "3");

    const prefixInput = screen.getByLabelText(/prefix/i);
    await user.type(prefixInput, "archive/");

    await user.click(screen.getByRole("button", { name: /add rule/i }));

    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith("photos", {
        kind: "noncurrent-expiration",
        noncurrent_days: 7,
        newer_noncurrent_versions: 3,
        prefix: "archive/",
      });
    });
  });

  it("selecting Abort incomplete multipart reveals days_after_initiation and submits correct payload", async () => {
    const spy = vi.spyOn(api, "createRule").mockResolvedValue({
      id: "rule-3",
      managed: true,
      kind: "abort-incomplete-multipart",
      days_after_initiation: 14,
      prefix: "",
    });
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateRuleDialog open={true} onOpenChange={() => {}} bucket="photos" />
      </Wrapper>,
    );

    // Open the kind select and pick Abort incomplete multipart
    const kindTrigger = screen.getByRole("combobox", { name: /rule kind/i });
    await user.click(kindTrigger);
    const option = await screen.findByRole("option", { name: /abort incomplete multipart/i });
    await user.click(option);

    // The days_after_initiation field should now be visible
    const daiInput = await screen.findByLabelText(/days after initiation/i);
    expect(daiInput).toBeInTheDocument();

    // expiration days field should NOT be visible
    expect(screen.queryByLabelText(/^days$/i)).not.toBeInTheDocument();

    await user.clear(daiInput);
    await user.type(daiInput, "14");

    await user.click(screen.getByRole("button", { name: /add rule/i }));

    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith("photos", {
        kind: "abort-incomplete-multipart",
        days_after_initiation: 14,
        prefix: "",
      });
    });
  });

  it("shows noncurrent versioning warning when versioningEnabled is false", async () => {
    vi.spyOn(api, "createRule").mockResolvedValue({
      id: "rule-4",
      managed: true,
      kind: "noncurrent-expiration",
      noncurrent_days: 7,
      newer_noncurrent_versions: 0,
      prefix: "",
    });
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateRuleDialog
          open={true}
          onOpenChange={() => {}}
          bucket="photos"
          versioningEnabled={false}
        />
      </Wrapper>,
    );

    // Switch to noncurrent-expiration kind
    const kindTrigger = screen.getByRole("combobox", { name: /rule kind/i });
    await user.click(kindTrigger);
    const option = await screen.findByRole("option", { name: /noncurrent versions/i });
    await user.click(option);

    // Warning should be visible
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(
      within(screen.getByRole("alert")).getByText(/versioning.*disabled/i),
    ).toBeInTheDocument();
  });

  it("maps pointer errors to the matching form field", async () => {
    const { AppError } = await import("@/lib/api/errors");
    vi.spyOn(api, "createRule").mockRejectedValue(
      new AppError({
        status: 422,
        code: "validation_error",
        message: "Must be positive",
        pointer: "/data/attributes/noncurrent_days",
      }),
    );
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateRuleDialog open={true} onOpenChange={() => {}} bucket="photos" />
      </Wrapper>,
    );

    // Switch to noncurrent-expiration kind
    const kindTrigger = screen.getByRole("combobox", { name: /rule kind/i });
    await user.click(kindTrigger);
    const option = await screen.findByRole("option", { name: /noncurrent versions/i });
    await user.click(option);

    await user.click(screen.getByRole("button", { name: /add rule/i }));

    await waitFor(() => {
      expect(screen.getByText("Must be positive")).toBeInTheDocument();
    });
  });

  it("non-blocking warning: versioningEnabled=false shows alert but still submits noncurrent-expiration", async () => {
    const spy = vi.spyOn(api, "createRule").mockResolvedValue({
      id: "rule-5",
      managed: true,
      kind: "noncurrent-expiration",
      noncurrent_days: 5,
      newer_noncurrent_versions: 2,
      prefix: "data/",
    });
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateRuleDialog
          open={true}
          onOpenChange={() => {}}
          bucket="photos"
          versioningEnabled={false}
        />
      </Wrapper>,
    );

    // Switch to noncurrent-expiration kind
    const kindTrigger = screen.getByRole("combobox", { name: /rule kind/i });
    await user.click(kindTrigger);
    const option = await screen.findByRole("option", { name: /noncurrent versions/i });
    await user.click(option);

    // Warning must be visible before submission
    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });

    // Fill in valid values
    const noncurrentDaysInput = await screen.findByLabelText(/noncurrent days/i);
    await user.clear(noncurrentDaysInput);
    await user.type(noncurrentDaysInput, "5");

    const newerVersionsInput = screen.getByLabelText(/newer noncurrent versions/i);
    await user.clear(newerVersionsInput);
    await user.type(newerVersionsInput, "2");

    const prefixInput = screen.getByLabelText(/prefix/i);
    await user.type(prefixInput, "data/");

    // Submit — the warning must NOT block the call
    await user.click(screen.getByRole("button", { name: /add rule/i }));

    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith("photos", {
        kind: "noncurrent-expiration",
        noncurrent_days: 5,
        newer_noncurrent_versions: 2,
        prefix: "data/",
      });
    });
  });

  it("pointer→field mapping: days_after_initiation maps to abort-mpu field; newer_noncurrent_versions maps to noncurrent field; unmatched pointer falls back to toast", async () => {
    const { AppError } = await import("@/lib/api/errors");

    // Case 1: /data/attributes/days_after_initiation on abort-incomplete-multipart
    {
      const spy = vi.spyOn(api, "createRule").mockRejectedValue(
        new AppError({
          status: 422,
          code: "validation_error",
          message: "Too large",
          pointer: "/data/attributes/days_after_initiation",
        }),
      );
      const user = userEvent.setup();
      const qc = makeQueryClient();
      const { unmount } = render(
        <Wrapper qc={qc}>
          <CreateRuleDialog open={true} onOpenChange={() => {}} bucket="photos" />
        </Wrapper>,
      );

      const kindTrigger = screen.getByRole("combobox", { name: /rule kind/i });
      await user.click(kindTrigger);
      const option = await screen.findByRole("option", { name: /abort incomplete multipart/i });
      await user.click(option);

      await user.click(screen.getByRole("button", { name: /add rule/i }));

      await waitFor(() => {
        expect(screen.getByText("Too large")).toBeInTheDocument();
      });

      spy.mockRestore();
      unmount();
      cleanup();
    }

    // Case 2: /data/attributes/newer_noncurrent_versions on noncurrent-expiration
    {
      const spy = vi.spyOn(api, "createRule").mockRejectedValue(
        new AppError({
          status: 422,
          code: "validation_error",
          message: "Too many versions",
          pointer: "/data/attributes/newer_noncurrent_versions",
        }),
      );
      const user = userEvent.setup();
      const qc = makeQueryClient();
      const { unmount } = render(
        <Wrapper qc={qc}>
          <CreateRuleDialog open={true} onOpenChange={() => {}} bucket="photos" />
        </Wrapper>,
      );

      const kindTrigger = screen.getByRole("combobox", { name: /rule kind/i });
      await user.click(kindTrigger);
      const option = await screen.findByRole("option", { name: /noncurrent versions/i });
      await user.click(option);

      await user.click(screen.getByRole("button", { name: /add rule/i }));

      await waitFor(() => {
        expect(screen.getByText("Too many versions")).toBeInTheDocument();
      });

      spy.mockRestore();
      unmount();
      cleanup();
    }

    // Case 3: unmatched pointer (e.g. /data/attributes/unknown_field) falls back to toast.
    // The Toaster is rendered inside a container that the Dialog marks aria-hidden, so we
    // query with { hidden: true } to find the toast element regardless.
    {
      vi.spyOn(api, "createRule").mockRejectedValue(
        new AppError({
          status: 422,
          code: "validation_error",
          message: "Unknown field error",
          pointer: "/data/attributes/unknown_field",
        }),
      );
      const user = userEvent.setup();
      const qc = makeQueryClient();
      render(
        <Wrapper qc={qc}>
          <CreateRuleDialog open={true} onOpenChange={() => {}} bucket="photos" />
        </Wrapper>,
      );

      await user.click(screen.getByRole("button", { name: /add rule/i }));

      await waitFor(() => {
        const statuses = screen.getAllByRole("status", { hidden: true });
        const text = statuses.map((n) => n.textContent ?? "").join(" ");
        expect(text).toContain("Unknown field error");
      });
    }
  });
});
