import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import { CURRENT_SELECTION_FIELD_PANEL_CLASS } from "../../base/components/detail-card-shared";
import type { EditableWorkTypeConfigurationState } from "../lib/detail-card-types";
import type { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeStatesList } from "./work-type-states-list";

export function WorkTypeReadySection({
  messages,
  state,
}: {
  messages: ReturnType<typeof getWorkTypeDetailMessages>;
  state: Extract<EditableWorkTypeConfigurationState, { status: "ready" }>;
}) {
  return (
    <div className="grid gap-2.5">
      <div className={CURRENT_SELECTION_FIELD_PANEL_CLASS}>
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
          {messages.workTypeNameLabel}
        </span>
        <p
          className={cn(
            "m-0 min-w-0 [overflow-wrap:anywhere]",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
        >
          {state.draft.name}
        </p>
      </div>
      <WorkTypeStatesList
        messages={messages}
        states={state.initialValues.states}
      />
    </div>
  );
}
