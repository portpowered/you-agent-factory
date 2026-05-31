import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import type { EditableWorkTypeValues } from "../../../current-factory-definition/lib/work-type-editable-values";
import {
  CurrentSelectionSectionHeader,
  WORKSTATION_SUMMARY_ITEM_CLASS,
} from "../../base/components/detail-card-shared";
import type { getWorkTypeDetailMessages } from "../messages/work-type-detail";

export function WorkTypeStatesList({
  messages,
  states,
}: {
  messages: ReturnType<typeof getWorkTypeDetailMessages>;
  states: EditableWorkTypeValues["states"];
}) {
  const headingId = "work-type-states-heading";

  return (
    <section aria-labelledby={headingId} className="mt-4 grid gap-2.5">
      <CurrentSelectionSectionHeader
        headingId={headingId}
        title={messages.statesHeading}
      />
      {states == null || states.length === 0 ? (
        <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.statesEmpty}
        </p>
      ) : (
        <ul className="m-0 grid list-none gap-2 p-0">
          {states.map((state) => (
            <li
              className={WORKSTATION_SUMMARY_ITEM_CLASS}
              key={`${state.name}:${state.type}`}
            >
              <div className="grid min-w-0 gap-1 sm:grid-cols-2 sm:gap-3">
                <div className="grid min-w-0 gap-1">
                  <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
                    {messages.stateNameColumnLabel}
                  </span>
                  <span
                    className={cn(
                      "min-w-0 [overflow-wrap:anywhere]",
                      DASHBOARD_BODY_TEXT_CLASS,
                    )}
                  >
                    {state.name}
                  </span>
                </div>
                <div className="grid min-w-0 gap-1">
                  <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
                    {messages.stateTypeColumnLabel}
                  </span>
                  <span
                    className={cn(
                      "min-w-0 [overflow-wrap:anywhere]",
                      DASHBOARD_SUPPORTING_TEXT_CLASS,
                    )}
                  >
                    {messages.localizeWorkStateType(state.type)}
                  </span>
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
