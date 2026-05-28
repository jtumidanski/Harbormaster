import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { downloadURL } from "./api";

// Text/JSON previews are Range-capped at 1 MiB so opening a huge log never
// slurps the whole thing — a partial text view is still useful. Images and
// PDFs cannot be rendered from a truncated byte range, so they are fetched in
// full up to IMAGE_PREVIEW_CAP_BYTES; larger ones prompt a download instead.
const PREVIEW_CAP_BYTES = 1024 * 1024;
const IMAGE_PREVIEW_CAP_BYTES = 25 * 1024 * 1024;

export type PreviewPaneProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  objectKey: string;
  contentType: string;
  size: number;
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

// MinIO often stores objects with an empty or octet-stream content type, so we
// fall back to the file extension to recognise images/PDFs that would otherwise
// be treated as un-previewable binaries.
const IMAGE_EXTENSIONS = new Set([
  "png",
  "jpg",
  "jpeg",
  "gif",
  "webp",
  "svg",
  "bmp",
  "ico",
  "avif",
  "tif",
  "tiff",
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
  if (IMAGE_EXTENSIONS.has(ext)) return "image";
  if (ext === "pdf") return "pdf";
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
  | { kind: "too_large" }
  | { kind: "error"; message: string };

export function PreviewPane({
  open,
  onOpenChange,
  bucket,
  objectKey,
  contentType,
  size,
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

    // Images/PDFs can't render from a truncated byte range, so fetch them whole
    // — but refuse oversized ones rather than pull tens of MiB into a blob.
    const wantsFull = flavour === "image" || flavour === "pdf";
    if (wantsFull && size > IMAGE_PREVIEW_CAP_BYTES) {
      setState({ kind: "too_large" });
      return;
    }

    const load = async () => {
      try {
        const res = await fetch(downloadURL(bucket, objectKey), {
          headers: wantsFull ? {} : { Range: `bytes=0-${PREVIEW_CAP_BYTES - 1}` },
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
  }, [open, bucket, objectKey, contentType, size]);

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
          {state.kind === "too_large" && (
            <p className="text-sm text-muted-foreground">
              File is larger than 25 MiB — download to view.
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
