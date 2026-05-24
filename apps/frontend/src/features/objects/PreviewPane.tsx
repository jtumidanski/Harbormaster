import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { downloadURL } from "./api";

// Cap preview fetches at 1 MiB via a Range header so opening a 4 GiB
// object never tries to slurp the whole thing. The backend honours the
// browser's Range request because S3 GetObject returns 206 partial
// content for it.
const PREVIEW_CAP_BYTES = 1024 * 1024;

export type PreviewPaneProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  objectKey: string;
  contentType: string;
};

type PreviewKind = "image" | "pdf" | "json" | "text" | "binary";

const TEXT_PREFIXES = ["text/"];
const TEXT_CONTENT_TYPES = new Set([
  "application/json",
  "application/ld+json",
  "application/xml",
  "application/yaml",
  "application/x-yaml",
  "application/x-sh",
  "application/javascript",
  "application/x-www-form-urlencoded",
]);

const TEXT_EXTENSIONS = new Set([
  "txt",
  "md",
  "log",
  "yaml",
  "yml",
  "sh",
  "bash",
  "ini",
  "conf",
  "csv",
  "tsv",
  "xml",
]);

function extOf(key: string): string {
  const i = key.lastIndexOf(".");
  return i >= 0 ? key.slice(i + 1).toLowerCase() : "";
}

function classify(contentType: string, key: string): PreviewKind {
  const ct = contentType.toLowerCase();
  if (ct.startsWith("image/")) return "image";
  if (ct === "application/pdf") return "pdf";
  if (ct === "application/json" || ct === "application/ld+json") return "json";
  const ext = extOf(key);
  if (ext === "json") return "json";
  if (TEXT_PREFIXES.some((p) => ct.startsWith(p))) return "text";
  if (TEXT_CONTENT_TYPES.has(ct)) return "text";
  if (TEXT_EXTENSIONS.has(ext)) return "text";
  return "binary";
}

type PreviewState =
  | { kind: "loading" }
  | { kind: "image" | "pdf"; blobUrl: string }
  | { kind: "text"; text: string }
  | { kind: "json"; text: string }
  | { kind: "binary" }
  | { kind: "error"; message: string };

export function PreviewPane({
  open,
  onOpenChange,
  bucket,
  objectKey,
  contentType,
}: PreviewPaneProps) {
  const [state, setState] = useState<PreviewState>({ kind: "loading" });

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    let blobUrlToRevoke: string | null = null;
    setState({ kind: "loading" });

    const flavour = classify(contentType, objectKey);
    if (flavour === "binary") {
      setState({ kind: "binary" });
      return;
    }

    const load = async () => {
      try {
        const res = await fetch(downloadURL(bucket, objectKey), {
          headers: { Range: `bytes=0-${PREVIEW_CAP_BYTES - 1}` },
          credentials: "include",
        });
        if (!res.ok && res.status !== 206) {
          throw new Error(`HTTP ${res.status}`);
        }
        if (flavour === "image" || flavour === "pdf") {
          const blob = await res.blob();
          const url = URL.createObjectURL(blob);
          blobUrlToRevoke = url;
          if (!cancelled) setState({ kind: flavour, blobUrl: url });
          return;
        }
        const raw = await res.text();
        if (flavour === "json") {
          try {
            const pretty = JSON.stringify(JSON.parse(raw), null, 2);
            if (!cancelled) setState({ kind: "json", text: pretty });
          } catch {
            // Not actually valid JSON — fall back to the raw text view
            // rather than failing the whole preview.
            if (!cancelled) setState({ kind: "text", text: raw });
          }
          return;
        }
        if (!cancelled) setState({ kind: "text", text: raw });
      } catch (err) {
        if (!cancelled)
          setState({
            kind: "error",
            message: err instanceof Error ? err.message : "Failed to load preview.",
          });
      }
    };
    void load();

    return () => {
      cancelled = true;
      if (blobUrlToRevoke) URL.revokeObjectURL(blobUrlToRevoke);
    };
  }, [open, bucket, objectKey, contentType]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="truncate font-mono text-sm">{objectKey}</DialogTitle>
          <DialogDescription>{contentType || "unknown content type"}</DialogDescription>
        </DialogHeader>
        <div className="min-h-[200px]">
          {state.kind === "loading" && (
            <p className="text-sm text-muted-foreground">Loading preview…</p>
          )}
          {state.kind === "error" && (
            <p className="text-sm text-destructive" role="alert">
              {state.message}
            </p>
          )}
          {state.kind === "binary" && (
            <p className="text-sm text-muted-foreground">
              No preview available — download to view.
            </p>
          )}
          {state.kind === "image" && (
            <img
              src={state.blobUrl}
              alt={objectKey}
              className="mx-auto max-h-[60vh] max-w-full object-contain"
            />
          )}
          {state.kind === "pdf" && (
            <embed
              src={state.blobUrl}
              type="application/pdf"
              className="h-[60vh] w-full"
              aria-label={`PDF preview of ${objectKey}`}
            />
          )}
          {(state.kind === "text" || state.kind === "json") && (
            <pre className="max-h-[60vh] overflow-auto rounded border bg-muted p-3 font-mono text-xs">
              {state.text}
            </pre>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
