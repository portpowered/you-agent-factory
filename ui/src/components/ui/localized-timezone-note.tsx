import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "./dashboard-typography";
import { formatLocalTimezoneContext } from "./formatters";
import { cn } from "../../lib/cn";

export function LocalizedTimezoneNote({
  children,
  className,
  locale,
  timezoneLabel,
}: {
  children: string;
  className?: string;
  locale?: string | null;
  timezoneLabel: string;
}) {
  return (
    <div
      className={cn(
        "grid gap-1 rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2 text-af-text-subtle",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
        className,
      )}
    >
      <p className="m-0">{children}</p>
      <p className="m-0">{formatLocalTimezoneContext(timezoneLabel, locale)}</p>
    </div>
  );
}
