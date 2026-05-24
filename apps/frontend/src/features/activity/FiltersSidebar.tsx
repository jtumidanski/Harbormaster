import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { AuditFilter } from "./types";

const TARGET_TYPES = [
  "bucket",
  "object",
  "user",
  "service_account",
  "admin",
  "session",
  "connection",
  "lifecycle_rule",
] as const;

const OUTCOMES = ["success", "failure"] as const;

const ANY_VALUE = "__any__";

function toLocalDateTimeInput(iso: string): string {
  // <input type="datetime-local"> expects YYYY-MM-DDTHH:MM (no Z).
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fromLocalDateTimeInput(v: string): string {
  if (!v) return "";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString();
}

export function FiltersSidebar({
  value,
  onApply,
  onReset,
}: {
  value: AuditFilter;
  onApply: (next: AuditFilter) => void;
  onReset: () => void;
}) {
  const [action, setAction] = useState(value.action ?? "");
  const [targetType, setTargetType] = useState(value.target_type ?? "");
  const [targetId, setTargetId] = useState(value.target_id ?? "");
  const [outcome, setOutcome] = useState(value.outcome ?? "");
  const [fromLocal, setFromLocal] = useState(toLocalDateTimeInput(value.from ?? ""));
  const [toLocal, setToLocal] = useState(toLocalDateTimeInput(value.to ?? ""));

  // Re-sync local fields when external filters change (e.g. URL nav).
  useEffect(() => {
    setAction(value.action ?? "");
    setTargetType(value.target_type ?? "");
    setTargetId(value.target_id ?? "");
    setOutcome(value.outcome ?? "");
    setFromLocal(toLocalDateTimeInput(value.from ?? ""));
    setToLocal(toLocalDateTimeInput(value.to ?? ""));
  }, [value]);

  function handleApply(e: React.FormEvent) {
    e.preventDefault();
    const next: AuditFilter = {};
    if (action.trim()) next.action = action.trim();
    if (targetType) next.target_type = targetType;
    if (targetId.trim()) next.target_id = targetId.trim();
    if (outcome) next.outcome = outcome;
    const fromIso = fromLocalDateTimeInput(fromLocal);
    const toIso = fromLocalDateTimeInput(toLocal);
    if (fromIso) next.from = fromIso;
    if (toIso) next.to = toIso;
    onApply(next);
  }

  function handleReset() {
    setAction("");
    setTargetType("");
    setTargetId("");
    setOutcome("");
    setFromLocal("");
    setToLocal("");
    onReset();
  }

  return (
    <form
      onSubmit={handleApply}
      className="space-y-4 rounded-lg border bg-card p-4 text-card-foreground shadow-sm"
      aria-label="Filters"
    >
      <h2 className="text-lg font-semibold">Filters</h2>

      <div className="space-y-2">
        <Label htmlFor="filter-action">Action</Label>
        <Input
          id="filter-action"
          value={action}
          onChange={(e) => setAction(e.target.value)}
          placeholder="e.g. bucket.create"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="filter-target-type">Target type</Label>
        <Select
          value={targetType === "" ? ANY_VALUE : targetType}
          onValueChange={(v) => setTargetType(v === ANY_VALUE ? "" : v)}
        >
          <SelectTrigger id="filter-target-type" aria-label="Target type">
            <SelectValue placeholder="Any" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_VALUE}>Any</SelectItem>
            {TARGET_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {t}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2">
        <Label htmlFor="filter-target-id">Target ID</Label>
        <Input
          id="filter-target-id"
          value={targetId}
          onChange={(e) => setTargetId(e.target.value)}
          placeholder="e.g. photos"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="filter-outcome">Outcome</Label>
        <Select
          value={outcome === "" ? ANY_VALUE : outcome}
          onValueChange={(v) => setOutcome(v === ANY_VALUE ? "" : v)}
        >
          <SelectTrigger id="filter-outcome" aria-label="Outcome">
            <SelectValue placeholder="Any" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_VALUE}>Any</SelectItem>
            {OUTCOMES.map((o) => (
              <SelectItem key={o} value={o}>
                {o}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-2">
        <Label htmlFor="filter-from">From</Label>
        <Input
          id="filter-from"
          type="datetime-local"
          value={fromLocal}
          onChange={(e) => setFromLocal(e.target.value)}
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="filter-to">To</Label>
        <Input
          id="filter-to"
          type="datetime-local"
          value={toLocal}
          onChange={(e) => setToLocal(e.target.value)}
        />
      </div>

      <div className="flex gap-2">
        <Button type="submit">Apply</Button>
        <Button type="button" variant="outline" onClick={handleReset}>
          Reset
        </Button>
      </div>
    </form>
  );
}
