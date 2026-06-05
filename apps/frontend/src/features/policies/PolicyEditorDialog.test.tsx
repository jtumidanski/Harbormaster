import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { PolicyEditorDialog } from "./PolicyEditorDialog";
import * as policiesApi from "./policiesApi";

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

describe("PolicyEditorDialog", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("CREATE mode: invalid JSON shows client-side error and does NOT call createPolicy", async () => {
    const spy = vi.spyOn(policiesApi, "createPolicy").mockResolvedValue(undefined);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <PolicyEditorDialog open={true} onOpenChange={() => {}} mode="create" />
      </Wrapper>,
    );

    const nameInput = screen.getByLabelText(/name/i);
    await user.type(nameInput, "my-policy");

    const documentTextarea = screen.getByLabelText(/document/i);
    await user.clear(documentTextarea);
    await user.paste("not valid json {");

    await user.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => {
      expect(screen.getByText("Document is not valid JSON.")).toBeInTheDocument();
    });

    expect(spy).not.toHaveBeenCalled();
  });

  it("CREATE mode: valid JSON + name calls createPolicy with parsed object", async () => {
    const spy = vi.spyOn(policiesApi, "createPolicy").mockResolvedValue(undefined);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <PolicyEditorDialog open={true} onOpenChange={() => {}} mode="create" />
      </Wrapper>,
    );

    const nameInput = screen.getByLabelText(/name/i);
    await user.type(nameInput, "my-policy");

    const documentTextarea = screen.getByLabelText(/document/i);
    await user.clear(documentTextarea);
    await user.paste(
      '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::*"]}]}',
    );

    await user.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith("my-policy", {
        Version: "2012-10-17",
        Statement: [{ Effect: "Allow", Action: ["s3:GetObject"], Resource: ["arn:aws:s3:::*"] }],
      });
    });
  });
});
