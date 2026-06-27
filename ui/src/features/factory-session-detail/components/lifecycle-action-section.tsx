import {
  DashboardActionButton,
  DashboardActionRow,
  DashboardLabel,
  DashboardText,
} from "../../../components/ui";
import { DetailCopy } from "../../../components/ui/widget-frame";
import type { FactorySessionLifecycleActionID } from "../lib/factory-session-lifecycle-controls";
import {
  type FactorySessionDetailMessages,
  getFactorySessionDetailMessages,
} from "../messages/factory-session-detail";

interface LifecycleActionSectionProps {
  availability: {
    actions: FactorySessionLifecycleActionID[];
    selectedDispatch?: { id: string };
    showDispatchSelectionHint: boolean;
    showEmptyState: boolean;
  };
  locale?: string;
}

export function LifecycleActionSection({
  availability,
  locale,
}: LifecycleActionSectionProps) {
  const messages = getFactorySessionDetailMessages(locale);

  return (
    <section className="grid gap-2">
      <DashboardLabel>{messages.lifecycleControlsHeading}</DashboardLabel>
      <DashboardActionRow
        actions={availability.actions.map((action) => (
          <DashboardActionButton disabled key={action} type="button">
            {lifecycleActionLabel(action, messages)}
          </DashboardActionButton>
        ))}
      />
      {availability.selectedDispatch ? (
        <DashboardText variant="supporting">
          {messages.lifecycleControlsSelectedDispatchLabel(
            availability.selectedDispatch.id,
          )}
        </DashboardText>
      ) : null}
      {availability.showDispatchSelectionHint ? (
        <DetailCopy>{messages.lifecycleControlsRetrySelectionHint}</DetailCopy>
      ) : null}
      {availability.showEmptyState ? (
        <DetailCopy>{messages.lifecycleControlsEmptyState}</DetailCopy>
      ) : null}
    </section>
  );
}

function lifecycleActionLabel(
  action: FactorySessionLifecycleActionID,
  messages: FactorySessionDetailMessages,
) {
  switch (action) {
    case "approve":
      return messages.lifecycleActionApproveLabel;
    case "cancel":
      return messages.lifecycleActionCancelLabel;
    case "pause":
      return messages.lifecycleActionPauseLabel;
    case "resume":
      return messages.lifecycleActionResumeLabel;
    case "retry-dispatch":
      return messages.lifecycleActionRetryDispatchLabel;
    case "terminate":
      return messages.lifecycleActionTerminateLabel;
  }
}
