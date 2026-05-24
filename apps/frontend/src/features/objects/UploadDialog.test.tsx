import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { PropsWithChildren } from "react";
import { Toaster } from "sonner";
import { UploadDialog } from "./UploadDialog";

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

// Minimal XMLHttpRequest stub: we only need open/send/upload-progress
// hooks so the dialog flow can fire and complete.
class XhrStub {
  static instances: XhrStub[] = [];
  status = 0;
  responseText = "";
  withCredentials = false;
  upload = { onprogress: null as ((ev: ProgressEvent) => void) | null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  openCalls: Array<{ method: string; url: string }> = [];
  sendCalls: unknown[] = [];
  headers: Record<string, string> = {};

  constructor() {
    XhrStub.instances.push(this);
  }

  open(method: string, url: string): void {
    this.openCalls.push({ method, url });
  }
  setRequestHeader(name: string, value: string): void {
    this.headers[name] = value;
  }
  send(body: unknown): void {
    this.sendCalls.push(body);
  }
  abort(): void {
    this.onabort?.();
  }

  complete(status: number, body = ""): void {
    this.status = status;
    this.responseText = body;
    this.onload?.();
  }
}

describe("UploadDialog", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    XhrStub.instances = [];
    document.cookie = "harbormaster_csrf=test-token; Path=/";
    vi.stubGlobal("XMLHttpRequest", XhrStub);
  });

  afterEach(() => {
    cleanup();
  });

  it("selecting a file under the cap kicks off a POST", async () => {
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <UploadDialog open onOpenChange={() => undefined} bucket="photos" prefix="" />
      </Wrapper>,
    );

    const input = screen.getByLabelText(/choose a file/i);
    const file = new File(["hello"], "hello.txt", { type: "text/plain" });
    await user.upload(input, file);

    await waitFor(() => expect(screen.getByText(/hello\.txt/)).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /^upload$/i }));

    await waitFor(() => expect(XhrStub.instances.length).toBe(1));
    const xhr = XhrStub.instances[0];
    expect(xhr.openCalls[0]?.method).toBe("POST");
    expect(xhr.openCalls[0]?.url).toBe("/api/v1/buckets/photos/objects");
    expect(xhr.sendCalls.length).toBe(1);
    expect(xhr.headers["X-CSRF-Token"]).toBe("test-token");
  });

  it("413 response with details.limit_bytes surfaces the dynamic cap (T3.28)", async () => {
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <UploadDialog open onOpenChange={() => undefined} bucket="photos" prefix="" />
      </Wrapper>,
    );

    const input = screen.getByLabelText(/choose a file/i);
    // Small payload — below the 100 MiB client-side gate so the POST
    // actually fires. The server then "responds" with a 413 carrying
    // a 200 MiB limit (operator configured a larger cap than the
    // client-side default expects).
    const file = new File(["small payload"], "ok.txt", { type: "text/plain" });
    await user.upload(input, file);
    await waitFor(() => expect(screen.getByText(/ok\.txt/)).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /^upload$/i }));
    await waitFor(() => expect(XhrStub.instances.length).toBe(1));

    const xhr = XhrStub.instances[0];
    xhr.complete(
      413,
      JSON.stringify({
        error: {
          code: "upload_too_large",
          details: { limit_bytes: 209715200 }, // 200 MiB
        },
      }),
    );

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(screen.getByRole("alert").textContent ?? "").toMatch(/200 MiB/);
  });

  it("selecting a file over the cap rejects client-side without POST", async () => {
    const user = userEvent.setup();
    const qc = makeQueryClient();
    render(
      <Wrapper qc={qc}>
        <UploadDialog open onOpenChange={() => undefined} bucket="photos" prefix="" />
      </Wrapper>,
    );

    const input = screen.getByLabelText(/choose a file/i);
    // 100 MiB + 1.
    const bigFile = new File([new Uint8Array(8)], "big.bin", { type: "application/octet-stream" });
    Object.defineProperty(bigFile, "size", { value: 100 * 1024 * 1024 + 1 });
    await user.upload(input, bigFile);

    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());
    expect(screen.getByRole("alert").textContent ?? "").toMatch(/exceeds the configured cap/i);
    // No XHR should have been created.
    expect(XhrStub.instances.length).toBe(0);
    // Upload button should remain disabled (no valid file selected).
    expect(screen.getByRole("button", { name: /^upload$/i })).toBeDisabled();
  });
});
