import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/ui/dashboard-typography";
import { cx } from "../../lib/cx";
import { getWorkflowActivityShellMessages } from "./messages/activity-shell";

const CURRENT_ACTIVITY_CARD_CLASS =
  "relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-[1.2rem] shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4";
const CURRENT_ACTIVITY_EYEBROW_CLASS =
  "mb-[0.65rem] text-xs font-bold uppercase tracking-[0.16em] text-af-accent";
const CURRENT_ACTIVITY_HEADER_CLASS = "mb-4";
const CURRENT_ACTIVITY_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);

export function EmptyCurrentActivityCard({ locale }: { locale?: string }) {
  const messages = getWorkflowActivityShellMessages(locale);

  return (
    <section
      aria-labelledby="workflow-graph-heading"
      className={CURRENT_ACTIVITY_CARD_CLASS}
    >
      <div className={CURRENT_ACTIVITY_HEADER_CLASS}>
        <div>
          <p className={CURRENT_ACTIVITY_EYEBROW_CLASS}>{messages.eyebrow}</p>
          <h2
            className={CURRENT_ACTIVITY_TITLE_CLASS}
            id="workflow-graph-heading"
          >
            {messages.title}
          </h2>
        </div>
      </div>
      <div className="grid min-h-60 items-start gap-[0.35rem] rounded-2xl border border-dashed border-af-overlay/15 bg-af-overlay/4 p-5 [&_h3]:m-0">
        <h3>{messages.emptyTitle}</h3>
        <p>{messages.emptyMessage}</p>
      </div>
    </section>
  );
}
