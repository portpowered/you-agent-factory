import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { ActionRow } from "@you-agent-factory/components/layout";
import { Label, Text } from "@you-agent-factory/components/primitives";
import {
  AlertPanel,
  AlertPanelText,
} from "../../../../components/ui/alert-panel";
import { DashboardActionButton } from "../../../../components/ui/dashboard-action-button";
import { DashboardStatusPill } from "../../../../components/ui/dashboard-status-pill";
import type { FactorySessionLifecycleActionID } from "../../lib/factory-session-lifecycle-controls";
import {
  getFactorySessionLifecycleActionLabel,
  type LifecycleControlFeedbackState,
  resolveFactorySessionLifecycleFeedbackDisplay,
} from "../../lib/lifecycle/factory-session-lifecycle-feedback";
import { getFactorySessionDetailMessages } from "../../messages/factory-session-detail";

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
  const isLifecycleRequestPending = pendingActionID !== null;

  return (
    <section className="grid gap-2">
      <Label>{messages.lifecycleControlsHeading}</Label>
      <ActionRow
        actions={availability.actions.map((action) => (
          <DashboardActionButton
            disabled={isLifecycleRequestPending && pendingActionID !== action}
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
        <Text variant="supporting">
          {messages.lifecycleControlsSelectedDispatchLabel(
            availability.selectedDispatch.id,
          )}
        </Text>
      ) : null}
      {availability.showDispatchSelectionHint ? (
        <WidgetDetailCopy>
          {messages.lifecycleControlsRetrySelectionHint}
        </WidgetDetailCopy>
      ) : null}
      {availability.showEmptyState ? (
        <WidgetDetailCopy>
          {messages.lifecycleControlsEmptyState}
        </WidgetDetailCopy>
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
