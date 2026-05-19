import { DASHBOARD_PANEL_SHELL_CLASS } from "../../components/ui/dashboard-shell";
import { cx } from "../../lib/cx";
import { getWorkflowActivityShellMessages } from "./messages/activity-shell";

const CURRENT_ACTIVITY_CARD_CLASS = cx(
  DASHBOARD_PANEL_SHELL_CLASS,
  "relative flex h-full min-h-0 min-w-0 flex-col p-4 md:p-5",
);

export function EmptyCurrentActivityCard({ locale }: { locale?: string }) {
  const messages = getWorkflowActivityShellMessages(locale);

  return (
    <section
      aria-label={messages.title}
      className={CURRENT_ACTIVITY_CARD_CLASS}
    >
      <div className="grid min-h-60 items-start gap-1 rounded-2xl border border-dashed border-af-overlay/15 bg-af-overlay/4 p-5 [&_h3]:m-0">
        <h3>{messages.emptyTitle}</h3>
        <p>{messages.emptyMessage}</p>
      </div>
    </section>
  );
}
