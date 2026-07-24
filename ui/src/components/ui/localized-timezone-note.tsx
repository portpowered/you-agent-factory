import { SurfacePanel } from "@you-agent-factory/components";
import { cn } from "../../lib/cn";
import { DASHBOARD_SUPPORTING_TEXT_CLASS } from "./dashboard-typography";
import { formatLocalTimezoneContext } from "./formatters";

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
    <SurfacePanel
      className={cn(
        "grid gap-1 text-af-text-subtle",
        DASHBOARD_SUPPORTING_TEXT_CLASS,
        className,
      )}
      padding="compact"
      radius="lg"
      surface="low"
    >
      <p className="m-0">{children}</p>
      <p className="m-0">{formatLocalTimezoneContext(timezoneLabel, locale)}</p>
    </SurfacePanel>
  );
}
