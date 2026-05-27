import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { AlertTriangle, Copy } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AppError } from "@/lib/api/errors";
import { createShareLink, type ShareLink } from "./api";

const TTL_PRESETS = [
  { value: "1800", label: "30 minutes" },
  { value: "3600", label: "1 hour" },
  { value: "86400", label: "24 hours" },
  { value: "604800", label: "7 days" },
  { value: "custom", label: "Custom (seconds)" },
] as const;

const DEFAULT_TTL = "604800"; // 7 days

export type ShareLinkDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucket: string;
  objectKey: string;
};

function formatRelative(iso: string): string {
  const target = new Date(iso).getTime();
  if (!Number.isFinite(target)) return iso;
  const delta = Math.max(0, target - Date.now());
  const minutes = Math.floor(delta / 60_000);
  if (minutes < 60) return `in ${minutes} minute${minutes === 1 ? "" : "s"}`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `in ${hours} hour${hours === 1 ? "" : "s"}`;
  const days = Math.floor(hours / 24);
  return `in ${days} day${days === 1 ? "" : "s"}`;
}

function formatAbsolute(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

export function ShareLinkDialog({ open, onOpenChange, bucket, objectKey }: ShareLinkDialogProps) {
  const [preset, setPreset] = useState<string>(DEFAULT_TTL);
  const [customSeconds, setCustomSeconds] = useState<string>("3600");
  const [link, setLink] = useState<ShareLink | null>(null);

  useEffect(() => {
    if (!open) {
      setPreset(DEFAULT_TTL);
      setCustomSeconds("3600");
      setLink(null);
    }
  }, [open]);

  const expiresSeconds = (): number => {
    if (preset === "custom") {
      const n = Number(customSeconds);
      return Number.isFinite(n) && n > 0 ? Math.trunc(n) : 0;
    }
    return Number(preset);
  };

  const mutation = useMutation({
    mutationFn: () => createShareLink(bucket, objectKey, expiresSeconds()),
    onSuccess: (sl) => {
      setLink(sl);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) toast.error(err.message || "Failed to create share link.");
      else toast.error("Failed to create share link.");
    },
  });

  const copyLink = async () => {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link.url);
      toast.success("Link copied to clipboard.");
    } catch {
      toast.error("Could not copy link.");
    }
  };

  const canSubmit = expiresSeconds() > 0 && !mutation.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Share link</DialogTitle>
          <DialogDescription>
            Generate a presigned URL for <span className="font-mono">{objectKey}</span>.
          </DialogDescription>
        </DialogHeader>

        {/* R17 warning — surfaced before the link is generated so the
            operator sees it while still choosing TTL, not just after. */}
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" aria-hidden="true" />
          <AlertTitle>Warning: this link cannot be revoked</AlertTitle>
          <AlertDescription>
            Anyone with the URL can access the object until it expires. If shared by mistake, you
            can disable the connection or rotate MinIO credentials.
          </AlertDescription>
        </Alert>

        {link === null ? (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="share-ttl">Expires after</Label>
              <Select value={preset} onValueChange={setPreset}>
                <SelectTrigger id="share-ttl">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TTL_PRESETS.map((p) => (
                    <SelectItem key={p.value} value={p.value}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {preset === "custom" && (
              <div className="space-y-1.5">
                <Label htmlFor="share-custom-seconds">Seconds</Label>
                <Input
                  id="share-custom-seconds"
                  type="number"
                  min={1}
                  value={customSeconds}
                  onChange={(e) => setCustomSeconds(e.target.value)}
                />
              </div>
            )}
          </div>
        ) : (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="share-link-url">URL</Label>
              <div className="flex items-center gap-2">
                <Input
                  id="share-link-url"
                  readOnly
                  value={link.url}
                  onFocus={(e) => e.currentTarget.select()}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  onClick={() => void copyLink()}
                  aria-label="Copy link"
                >
                  <Copy className="h-4 w-4" aria-hidden="true" />
                </Button>
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              Expires {formatRelative(link.expires_at)} ({formatAbsolute(link.expires_at)})
            </p>
          </div>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          {link === null && (
            <Button type="button" onClick={() => mutation.mutate()} disabled={!canSubmit}>
              {mutation.isPending ? "Creating…" : "Create link"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
