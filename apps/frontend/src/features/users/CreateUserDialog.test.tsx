import "@testing-library/jest-dom/vitest";
import { describe, it, expect, afterEach, beforeEach, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { CreateUserDialog } from "./CreateUserDialog";

type StubResponse = {
  match: (url: string, init?: RequestInit) => boolean;
  response: () => Response;
};

function installFetch(stubs: StubResponse[]) {
  const spy = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    for (const s of stubs) {
      if (s.match(url, init)) return Promise.resolve(s.response());
    }
    return Promise.resolve(
      new Response(JSON.stringify({ errors: [{ code: "not_found" }] }), {
        status: 404,
        headers: { "Content-Type": "application/vnd.api+json" },
      }),
    );
  });
  vi.stubGlobal("fetch", spy);
  return spy;
}

function jsonapi(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/vnd.api+json" },
  });
}

function templatesStub(): StubResponse {
  return {
    match: (u, init) => u.includes("/api/v1/policy-templates") && (init?.method ?? "GET") === "GET",
    response: () =>
      jsonapi({
        data: [
          {
            type: "policy_templates",
            id: "readonly",
            attributes: {
              name: "readonly",
              description: "Read-only access.",
              params_schema: null,
            },
          },
        ],
      }),
  };
}

function createUserStub(): StubResponse {
  return {
    match: (u, init) => u.endsWith("/api/v1/users") && (init?.method ?? "GET") === "POST",
    response: () =>
      jsonapi({
        data: {
          type: "users",
          id: "alice",
          attributes: {
            access_key: "alice",
            status: "enabled",
            attached_templates: [],
            other_policies: [],
            secret_key: "super-secret-value-xyz",
          },
        },
      }),
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
      <MemoryRouter>
        {children}
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

function installClipboard(): { writeText: ReturnType<typeof vi.fn> } {
  const writeText = vi.fn(() => Promise.resolve());
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  return { writeText };
}

describe("CreateUserDialog", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "harbormaster_csrf=test-token; Path=/";
  });

  afterEach(() => {
    cleanup();
  });

  it("hides the secret behind a mask after successful create", async () => {
    installFetch([templatesStub(), createUserStub()]);
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateUserDialog open={true} onOpenChange={() => {}} />
      </Wrapper>,
    );

    await user.type(await screen.findByLabelText(/access key/i), "alice");
    await user.click(screen.getByRole("button", { name: /^create user$/i }));

    await waitFor(() => {
      expect(screen.getByText(/one-time secret/i)).toBeInTheDocument();
    });

    // The secret should be masked by default — the raw secret should not appear in the DOM.
    expect(screen.queryByText(/super-secret-value-xyz/)).not.toBeInTheDocument();
    const display = screen.getByLabelText("secret-key-display");
    expect(display.textContent ?? "").not.toContain("super-secret-value-xyz");
    expect((display.textContent ?? "").length).toBeGreaterThan(0);
  });

  it("calls navigator.clipboard.writeText with the secret when Copy is clicked", async () => {
    installFetch([templatesStub(), createUserStub()]);
    const user = userEvent.setup();
    // Install our spy AFTER userEvent.setup so it isn't shadowed by user-event's own clipboard stub.
    const { writeText } = installClipboard();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <CreateUserDialog open={true} onOpenChange={() => {}} />
      </Wrapper>,
    );

    await user.type(await screen.findByLabelText(/access key/i), "alice");
    await user.click(screen.getByRole("button", { name: /^create user$/i }));

    await waitFor(() => {
      expect(screen.getByText(/one-time secret/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /^copy$/i }));

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith("super-secret-value-xyz");
    });
  });
});
