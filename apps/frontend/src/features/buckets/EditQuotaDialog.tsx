import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { AppError } from "@/lib/api/errors";
import { bucketsKeys } from "@/lib/api/keys";
import { setBucketQuota } from "./api";
import type { Quota, QuotaKind } from "./types";

type KindChoice = QuotaKind | "none";
type Unit = "MiB" | "GiB" | "TiB";

const UNIT_MULTIPLIERS: Record<Unit, bigint> = {
  MiB: 1024n * 1024n,
  GiB: 1024n * 1024n * 1024n,
  TiB: 1024n * 1024n * 1024n * 1024n,
};

const FIFO_BLOCKED_COPY =
  "FIFO quotas require versioning to be off; disable versioning in bucket settings first.";

function bytesToValueUnit(bytes: number): { value: string; unit: Unit } {
  if (bytes <= 0) return { value: "", unit: "GiB" };
  const tib = Number(UNIT_MULTIPLIERS.TiB);
  const gib = Number(UNIT_MULTIPLIERS.GiB);
  const mib = Number(UNIT_MULTIPLIERS.MiB);
  if (bytes % tib === 0) return { value: String(bytes / tib), unit: "TiB" };
  if (bytes % gib === 0) return { value: String(bytes / gib), unit: "GiB" };
  if (bytes % mib === 0) return { value: String(bytes / mib), unit: "MiB" };
  return { value: (bytes / gib).toFixed(2), unit: "GiB" };
}

function quotaToBytes(value: string, unit: Unit): number {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) return 0;
  const big = BigInt(Math.trunc(n)) * UNIT_MULTIPLIERS[unit];
  const cap = BigInt(Number.MAX_SAFE_INTEGER);
  return Number(big > cap ? cap : big);
}

export type EditQuotaDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bucketName: string;
  currentQuota: Quota | null;
  versioningEnabled: boolean;
};

export function EditQuotaDialog({
  open,
  onOpenChange,
  bucketName,
  currentQuota,
  versioningEnabled,
}: EditQuotaDialogProps) {
  const qc = useQueryClient();
  const [kind, setKind] = useState<KindChoice>(currentQuota?.kind ?? "none");
  const initialValueUnit = bytesToValueUnit(currentQuota?.bytes ?? 0);
  const [value, setValue] = useState(initialValueUnit.value);
  const [unit, setUnit] = useState<Unit>(initialValueUnit.unit);

  useEffect(() => {
    if (open) {
      setKind(currentQuota?.kind ?? "none");
      const next = bytesToValueUnit(currentQuota?.bytes ?? 0);
      setValue(next.value);
      setUnit(next.unit);
    }
  }, [open, currentQuota]);

  const mutation = useMutation({
    mutationFn: () => {
      if (kind === "none") return setBucketQuota(bucketName, "none");
      return setBucketQuota(bucketName, kind, quotaToBytes(value, unit));
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: bucketsKeys.detail(bucketName) });
      await qc.invalidateQueries({ queryKey: bucketsKeys.all() });
      toast.success("Quota updated.");
      onOpenChange(false);
    },
    onError: (err: unknown) => {
      if (err instanceof AppError) {
        if (err.code === "fifo_requires_versioning_off") {
          toast.error(FIFO_BLOCKED_COPY);
          return;
        }
        toast.error(err.message || "Failed to update quota.");
        return;
      }
      toast.error("Failed to update quota.");
    },
  });

  const submitDisabled =
    mutation.isPending ||
    (kind !== "none" && (!Number.isFinite(Number(value)) || Number(value) <= 0));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit quota</DialogTitle>
          <DialogDescription>Limit the total bytes stored in this bucket.</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
          className="space-y-4"
          noValidate
        >
          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">Kind</legend>
            <TooltipProvider delayDuration={150}>
              <div className="flex flex-col gap-2">
                <label className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="quota-kind"
                    value="none"
                    checked={kind === "none"}
                    onChange={() => setKind("none")}
                  />
                  <span>No quota</span>
                </label>
                <label className="flex items-center gap-2">
                  <input
                    type="radio"
                    name="quota-kind"
                    value="hard"
                    checked={kind === "hard"}
                    onChange={() => setKind("hard")}
                  />
                  <span>Hard (reject writes once exceeded)</span>
                </label>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <label
                      className={
                        versioningEnabled
                          ? "flex cursor-not-allowed items-center gap-2 opacity-60"
                          : "flex items-center gap-2"
                      }
                    >
                      <input
                        type="radio"
                        name="quota-kind"
                        value="fifo"
                        checked={kind === "fifo"}
                        disabled={versioningEnabled}
                        onChange={() => setKind("fifo")}
                      />
                      <span>FIFO (delete oldest objects to make room)</span>
                    </label>
                  </TooltipTrigger>
                  {versioningEnabled && <TooltipContent>{FIFO_BLOCKED_COPY}</TooltipContent>}
                </Tooltip>
              </div>
            </TooltipProvider>
          </fieldset>

          {kind !== "none" && (
            <div className="grid grid-cols-2 gap-2">
              <div className="space-y-2">
                <Label htmlFor="quota-value">Value</Label>
                <Input
                  id="quota-value"
                  type="number"
                  min="0"
                  step="any"
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="quota-unit">Unit</Label>
                <Select value={unit} onValueChange={(v) => setUnit(v as Unit)}>
                  <SelectTrigger id="quota-unit">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="MiB">MiB</SelectItem>
                    <SelectItem value="GiB">GiB</SelectItem>
                    <SelectItem value="TiB">TiB</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitDisabled}>
              {mutation.isPending ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
