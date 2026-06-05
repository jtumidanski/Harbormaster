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
import type { PolicyDetail } from "./types";

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

  it("EDIT mode: prefills name (disabled) + document from server; submit calls updatePolicy with parsed object", async () => {
    const policyDoc = { Version: "2012-10-17", Statement: [] };
    const mockDetail: PolicyDetail = {
      name: "existing-policy",
      origin: "custom",
      editable: true,
      statement_summary: "",
      document: policyDoc,
    };
    vi.spyOn(policiesApi, "getPolicy").mockResolvedValue(mockDetail);
    const updateSpy = vi.spyOn(policiesApi, "updatePolicy").mockResolvedValue(undefined);

    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <PolicyEditorDialog
          open={true}
          onOpenChange={() => {}}
          mode="edit"
          policyName="existing-policy"
        />
      </Wrapper>,
    );

    // Name input must be disabled in edit mode and prefilled once query resolves
    const nameInput = await screen.findByLabelText(/name/i);
    expect(nameInput).toBeDisabled();
    await waitFor(() => {
      expect(nameInput).toHaveValue("existing-policy");
    });

    // Document textarea must be prefilled with pretty-printed JSON
    const documentTextarea = await screen.findByLabelText(/document/i);
    await waitFor(() => {
      expect(documentTextarea).toHaveValue(JSON.stringify(policyDoc, null, 2));
    });

    // Change the document to different valid JSON
    const updatedDoc = {
      Version: "2012-10-17",
      Statement: [{ Effect: "Deny", Action: ["s3:*"], Resource: ["*"] }],
    };
    await user.clear(documentTextarea);
    await user.paste(JSON.stringify(updatedDoc));

    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith("existing-policy", updatedDoc);
    });
  });
});
