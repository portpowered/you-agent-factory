import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "./dashboard-typography";
import { cn } from "../../lib/cn";

export function LocalizedTimezoneNote({
  children,
  className,
}: {
  children: string;
  className?: string;
}) {
  return (
    <p
      className={cn(
        "m-0 rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2 text-af-text-subtle",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
        className,
      )}
    >
      {children}
    </p>
  );
}
