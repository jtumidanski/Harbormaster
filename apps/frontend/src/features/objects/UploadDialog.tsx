import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { readCsrfCookie } from "@/lib/api/csrf";
import { objectsKeys } from "@/lib/api/keys";

// V1 hardcodes the upload cap at 100 MiB to match the backend default of
// HARBORMASTER_UPLOAD_MAX_BYTES. T3.27 will surface this dynamically
// from /api/v1/config so the dialog matches whatever the operator has
// configured. The client-side check (pickFile) still uses this constant
// as a first line of defence; if it disagrees with the operator's
// configured cap, the server's 413 response carries `details.limit_bytes`
// which we surface in the rejection message instead (T3.28).
const UPLOAD_CAP_BYTES = 100 * 1024 * 1024;
const UPLOAD_CAP_LABEL = "100 MiB";

const OVER_CAP_MESSAGE = `This file exceeds the configured cap (${UPLOAD_CAP_LABEL}). Use \`mc cp\` or another direct S3 client.`;

// formatMiB renders a byte count as a human-readable MiB string. We
// drop the decimal for values ≥100 MiB (whole-number precision is
// enough at that scale) and trim the trailing ".0" so "100.0" surfaces
// as "100" — matching how operators tend to write the cap.
function formatMiB(bytes: number): string {
  const mib = bytes / 1024 / 1024;
  return mib >= 100 ? `${Math.round(mib)}` : mib.toFixed(1).replace(/\.0$/, "");
}

// overCapMessageFromServer builds the rejection message for a 413
// upload_too_large response. When the server includes the structured
// `details.limit_bytes` field we surface the operator-configured cap
// dynamically; otherwise we fall back to the hardcoded 100 MiB string
// so the message is never blank.
function overCapMessageFromServer(limitBytes: unknown): string {
  if (typeof limitBytes === "number" && Number.isFinite(limitBytes) && limitBytes > 0) {
    return `Upload exceeds the configured cap (${formatMiB(limitBytes)} MiB). Use \`mc cp\` or another direct S3 client.`;
  }
  return OVER_CAP_MESSAGE;
}

export type UploadDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  prefix: string;
};

type UploadState =
  | { kind: "idle" }
  | { kind: "uploading"; loaded: number; total: number }
  | { kind: "error"; message: string };

export function UploadDialog({ open, onOpenChange, bucket, prefix }: UploadDialogProps) {
  const qc = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [state, setState] = useState<UploadState>({ kind: "idle" });
  const [dragOver, setDragOver] = useState(false);
  const xhrRef = useRef<XMLHttpRequest | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!open) {
      // Reset state between sessions and abort any in-flight upload so
      // the next open starts clean.
      if (xhrRef.current) {
        try {
          xhrRef.current.abort();
        } catch {
          /* noop */
        }
        xhrRef.current = null;
      }
      setFile(null);
      setState({ kind: "idle" });
      setDragOver(false);
    }
  }, [open]);

  const pickFile = (f: File | null) => {
    if (!f) return;
    if (f.size > UPLOAD_CAP_BYTES) {
      setFile(null);
      setState({ kind: "error", message: OVER_CAP_MESSAGE });
      return;
    }
    setFile(f);
    setState({ kind: "idle" });
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    pickFile(e.target.files?.[0] ?? null);
  };

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    setDragOver(false);
    pickFile(e.dataTransfer.files?.[0] ?? null);
  };

  const submit = () => {
    if (!file) return;
    const key = `${prefix}${file.name}`;
    const fd = new FormData();
    fd.set("key", key);
    fd.set("content_type", file.type || "application/octet-stream");
    fd.set("file", file);

    const xhr = new XMLHttpRequest();
    xhrRef.current = xhr;
    xhr.open("POST", `/api/v1/buckets/${encodeURIComponent(bucket)}/objects`, true);
    xhr.withCredentials = true;
    const csrf = readCsrfCookie();
    if (csrf) xhr.setRequestHeader("X-CSRF-Token", csrf);
    xhr.setRequestHeader("Accept", "application/vnd.api+json, application/json");

    xhr.upload.onprogress = (ev) => {
      if (!ev.lengthComputable) return;
      setState({ kind: "uploading", loaded: ev.loaded, total: ev.total });
    };

    xhr.onload = () => {
      xhrRef.current = null;
      if (xhr.status >= 200 && xhr.status < 300) {
        void qc.invalidateQueries({ queryKey: objectsKeys.list(bucket, prefix) });
        toast.success("Upload complete.");
        onOpenChange(false);
        return;
      }
      if (xhr.status === 413) {
        // Prefer the server-reported limit (operator-configured cap) over
        // the hardcoded 100 MiB constant. The backend ships the value as
        // `details.limit_bytes` on the action-shape envelope; if the
        // payload is missing or malformed we fall back to the static
        // message so the operator never sees a blank error.
        let limitBytes: unknown;
        try {
          const parsed = JSON.parse(xhr.responseText) as {
            error?: { details?: { limit_bytes?: unknown } };
            errors?: { meta?: { limit_bytes?: unknown } }[];
          };
          limitBytes = parsed.error?.details?.limit_bytes ?? parsed.errors?.[0]?.meta?.limit_bytes;
        } catch {
          /* fall through to hardcoded message */
        }
        setState({ kind: "error", message: overCapMessageFromServer(limitBytes) });
        return;
      }
      // Try to surface the server's JSON:API message when present;
      // fall back to a generic string.
      let msg = `Upload failed (HTTP ${xhr.status}).`;
      try {
        const parsed = JSON.parse(xhr.responseText) as {
          errors?: { detail?: string; title?: string }[];
        };
        const first = parsed.errors?.[0];
        if (first?.detail || first?.title) msg = first.detail ?? first.title ?? msg;
      } catch {
        /* keep generic msg */
      }
      setState({ kind: "error", message: msg });
    };

    xhr.onerror = () => {
      xhrRef.current = null;
      setState({ kind: "error", message: "Network error during upload." });
    };

    xhr.onabort = () => {
      xhrRef.current = null;
    };

    setState({ kind: "uploading", loaded: 0, total: file.size });
    xhr.send(fd);
  };

  const uploading = state.kind === "uploading";
  const progressPct =
    state.kind === "uploading" && state.total > 0
      ? Math.min(100, Math.round((state.loaded / state.total) * 100))
      : 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Upload object</DialogTitle>
          <DialogDescription>
            Files larger than {UPLOAD_CAP_LABEL} must be uploaded via{" "}
            <span className="font-mono">mc cp</span> or another S3 client.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={handleDrop}
            className={`flex flex-col items-center justify-center gap-2 rounded border-2 border-dashed p-6 text-sm transition-colors ${
              dragOver ? "border-primary bg-accent/40" : "border-input bg-background"
            }`}
            data-testid="upload-dropzone"
          >
            <p className="text-muted-foreground">Drag a file here, or</p>
            <Label
              htmlFor="upload-file-input"
              className="cursor-pointer text-primary hover:underline"
            >
              choose a file
            </Label>
            <input
              ref={inputRef}
              id="upload-file-input"
              type="file"
              className="sr-only"
              onChange={handleInputChange}
            />
            {file && (
              <p className="text-foreground">
                <span className="font-mono">{file.name}</span> ({file.size.toLocaleString()} bytes)
              </p>
            )}
          </div>

          {uploading && (
            <div className="space-y-1">
              <div
                role="progressbar"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={progressPct}
                className="h-2 w-full overflow-hidden rounded bg-muted"
              >
                <div
                  className="h-full bg-primary transition-all"
                  style={{ width: `${progressPct}%` }}
                />
              </div>
              <p className="text-xs text-muted-foreground">{progressPct}%</p>
            </div>
          )}

          {state.kind === "error" && (
            <p className="text-sm text-destructive" role="alert">
              {state.message}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={uploading}
          >
            Cancel
          </Button>
          <Button type="button" onClick={submit} disabled={!file || uploading}>
            {uploading ? "Uploading…" : "Upload"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
