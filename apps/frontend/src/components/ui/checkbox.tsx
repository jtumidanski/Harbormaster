import * as React from "react";

import { cn } from "@/lib/utils";

export type CheckboxProps = React.InputHTMLAttributes<HTMLInputElement>;

// Checkbox is a thin, dependency-free wrapper over a native checkbox input
// styled to match the shadcn/ui surface. We use a native input (rather than
// @radix-ui/react-checkbox, which is not a project dependency) so callers
// get standard `checked` / `onChange` semantics with zero new packages.
const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, ...props }, ref) => (
    <input
      type="checkbox"
      ref={ref}
      className={cn(
        "h-4 w-4 shrink-0 cursor-pointer rounded border border-input accent-primary",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Checkbox.displayName = "Checkbox";

export { Checkbox };
