import { EnumSelect } from "@you-agent-factory/components/forms";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { WorkerSessionObservation } from "../../../api/worker-sessions";
import { AlertPanel } from "../../../components/ui/alert-panel";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import type { UseWorkerSessionTimelineTargetResult } from "../hooks/useWorkerSessionTimelineTarget";
import type { WorkerSessionTimelineMessages } from "../messages/worker-session-timeline";

export interface WorkerSessionTimelineTargetProps {
  messages: WorkerSessionTimelineMessages;
  state: UseWorkerSessionTimelineTargetResult;
  workID: string | null;
}

export function WorkerSessionTimelineTarget({
  messages,
  state,
  workID,
}: WorkerSessionTimelineTargetProps) {
  if (workID === null || state.status === "idle") {
    return (
      <WidgetDetailCopy>{messages.workSelectionRequired}</WidgetDetailCopy>
    );
  }

  if (state.status === "loading") {
    return (
      <WidgetDetailCopy aria-busy="true" role="status">
        {messages.sessionTargetLoading}
      </WidgetDetailCopy>
    );
  }

  if (state.status === "error") {
    return (
      <AlertPanel role="alert" tone="danger">
        <div className="grid gap-3">
          <WidgetDetailCopy>
            {state.error?.message ?? messages.sessionTargetError}
          </WidgetDetailCopy>
          <div>
            <DashboardActionButton onClick={state.refetch} type="button">
              {messages.sessionTargetRetry}
            </DashboardActionButton>
          </div>
        </div>
      </AlertPanel>
    );
  }

  if (state.observations.length === 0) {
    return <WidgetDetailCopy>{messages.sessionTargetEmpty}</WidgetDetailCopy>;
  }

  const selectedWorkerSessionID =
    state.selectedWorkerSessionID ?? state.observations[0]?.workerSessionId;

  return (
    <EnumSelect
      aria-label={messages.sessionTargetSelectLabel}
      id="worker-session-timeline-target"
      onValueChange={state.setSelectedWorkerSessionID}
      options={state.observations.map((observation) => ({
        label: sessionTargetOption(messages, observation),
        value: observation.workerSessionId,
      }))}
      value={selectedWorkerSessionID}
    />
  );
}

function sessionTargetOption(
  messages: WorkerSessionTimelineMessages,
  observation: WorkerSessionObservation,
): string {
  return messages.sessionTargetOption(
    observation.workerSessionId,
    observation.attemptId,
    observation.state,
  );
}
