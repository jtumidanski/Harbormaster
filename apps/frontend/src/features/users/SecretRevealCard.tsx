import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

const MASK = "•".repeat(40);

export type SecretRevealCardProps = {
  secret: string;
  onAcknowledge: () => void;
};

export function SecretRevealCard({ secret, onAcknowledge }: SecretRevealCardProps) {
  const [revealed, setRevealed] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(secret);
      toast.success("Secret copied to clipboard.");
    } catch {
      toast.error("Failed to copy secret. Copy it manually.");
    }
  }

  return (
    <div className="space-y-4">
      <Alert variant="destructive">
        <AlertTitle>One-time secret</AlertTitle>
        <AlertDescription>
          This is the only time you&apos;ll see this secret. Store it now in your password manager.
          You will not be able to retrieve it again.
        </AlertDescription>
      </Alert>

      <div className="space-y-2">
        <div className="text-xs uppercase text-muted-foreground">Secret key</div>
        <code
          aria-label="secret-key-display"
          className="block break-all rounded-md border bg-muted px-3 py-2 font-mono text-sm"
        >
          {revealed ? secret : MASK}
        </code>
        <div className="flex gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => setRevealed((v) => !v)}>
            {revealed ? "Hide" : "Show"}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              void copy();
            }}
          >
            Copy
          </Button>
        </div>
      </div>

      <div className="flex justify-end">
        <Button type="button" onClick={onAcknowledge}>
          I&apos;ve saved it
        </Button>
      </div>
    </div>
  );
}
