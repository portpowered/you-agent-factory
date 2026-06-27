import {
  AlertPanel,
  AlertPanelText,
  DashboardActionButton,
  DashboardActionRow,
  DashboardLabel,
  DashboardStatusPill,
  DashboardText,
} from "../../../../components/ui";
import { DetailCopy } from "../../../../components/ui/widget-frame";
import {
  type LifecycleControlFeedbackState,
  getFactorySessionLifecycleActionLabel,
  resolveFactorySessionLifecycleFeedbackDisplay,
} from "../../lib/lifecycle/factory-session-lifecycle-feedback";
import type { FactorySessionLifecycleActionID } from "../../lib/factory-session-lifecycle-controls";
import {
  getFactorySessionDetailMessages,
} from "../../messages/factory-session-detail";

interface LifecycleActionSectionProps {
  availability: {
    actions: FactorySessionLifecycleActionID[];
    selectedDispatch?: { id: string };
    showDispatchSelectionHint: boolean;
    showEmptyState: boolean;
  };
  feedback: LifecycleControlFeedbackState | null;
  locale?: string;
  onAction: (action: FactorySessionLifecycleActionID) => void;
  pendingActionID: FactorySessionLifecycleActionID | null;
}

export function LifecycleActionSection({
  availability,
  feedback,
  locale,
  onAction,
  pendingActionID,
}: LifecycleActionSectionProps) {
  const messages = getFactorySessionDetailMessages(locale);

  return (
    <section className="grid gap-2">
      <DashboardLabel>{messages.lifecycleControlsHeading}</DashboardLabel>
      <DashboardActionRow
        actions={availability.actions.map((action) => (
          <DashboardActionButton
            executing={pendingActionID === action}
            key={action}
            onClick={() => {
              void onAction(action);
            }}
            type="button"
          >
            {getFactorySessionLifecycleActionLabel(action, messages)}
          </DashboardActionButton>
        ))}
      />
      {feedback ? (
        <LifecycleControlFeedback feedback={feedback} locale={locale} />
      ) : null}
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

function LifecycleControlFeedback({
  feedback,
  locale,
}: {
  feedback: LifecycleControlFeedbackState;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const display = resolveFactorySessionLifecycleFeedbackDisplay(
    feedback,
    messages,
  );

  return (
    <AlertPanel role={display.role} tone={display.tone}>
      <div className="flex flex-wrap items-center gap-2">
        <DashboardStatusPill role="status" size="compact" tone={display.tone}>
          {display.outcomeLabel}
        </DashboardStatusPill>
        <AlertPanelText as="span">{display.title}</AlertPanelText>
      </div>
      {display.detail ? (
        <AlertPanelText variant="supporting">{display.detail}</AlertPanelText>
      ) : null}
    </AlertPanel>
  );
}
