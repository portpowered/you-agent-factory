import { Label, Text } from "@you-agent-factory/components/primitives";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import type { WorkerSessionObservation } from "../../../api/worker-sessions";
import { AlertPanel } from "../../../components/ui/alert-panel";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { DurabilityConfirmationState } from "../../../components/ui/durability-confirmation-state";
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
    <ul
      aria-label={messages.sessionTargetListLabel}
      className="m-0 grid list-none gap-3 p-0"
    >
      {state.observations.map((observation) => {
        const selected =
          observation.workerSessionId === selectedWorkerSessionID;
        return (
          <li
            className="grid min-w-0 gap-3 rounded-lg border border-outline bg-surface-container-low p-3"
            key={observation.workerSessionId}
          >
            <TargetObservationDetails
              messages={messages}
              observation={observation}
            />
            <DashboardActionButton
              aria-label={messages.openWorkerSessionTargetLabel(
                observation.workerSessionId,
                workID,
              )}
              aria-pressed={selected}
              className="w-fit"
              onClick={() =>
                state.setSelectedWorkerSessionID(observation.workerSessionId)
              }
              type="button"
            >
              {selected
                ? messages.selectedWorkerSessionAction
                : messages.openWorkerSessionAction}
            </DashboardActionButton>
          </li>
        );
      })}
    </ul>
  );
}

function TargetObservationDetails({
  messages,
  observation,
}: {
  messages: WorkerSessionTimelineMessages;
  observation: WorkerSessionObservation;
}) {
  return (
    <dl className="grid min-w-0 gap-2 sm:grid-cols-2">
      <TargetDetail
        label={messages.workLabel}
        value={workOrigin(messages, observation)}
      />
      <TargetDetail
        label={messages.workIDsLabel}
        value={
          observation.workIds.length > 0
            ? observation.workIds.join(", ")
            : messages.unknownValueLabel
        }
      />
      <TargetDetail
        label={messages.workerSessionIDLabel}
        value={observation.workerSessionId}
      />
      <TargetDetail
        label={messages.attemptDispatchIDLabel}
        value={observation.attemptId}
      />
      <TargetDetail
        label={messages.modelLabel}
        value={firstNonEmpty(observation.model) ?? messages.modelUnavailable}
      />
      {observation.reasoningEffort ? (
        <TargetDetail
          label={messages.reasoningEffortLabel}
          value={observation.reasoningEffort}
        />
      ) : null}
      <TargetDetail
        label={messages.sessionLifecycleLabel}
        value={messages.sessionLifecycleStateLabel(observation.state)}
      />
      <TargetDetail
        label={messages.durationLabel}
        value={messages.durationValue(observation.durationMillis)}
      />
      <TargetDetail
        label={messages.durationBasisLabel}
        value={observation.durationBasis}
      />
      <TargetDetail
        label={messages.providerSessionLabel}
        value={providerSessionOrigin(messages, observation)}
      />
      {observation.failure ? (
        <TargetDetail
          label={messages.failureSummaryLabel}
          value={
            firstNonEmpty(
              observation.failure.detail,
              observation.failure.kind,
            ) ?? messages.unknownValueLabel
          }
        />
      ) : null}
      <TargetDetail
        label={messages.recordingHealthTargetLabel}
        value={recordingHealthValue(messages, observation)}
      />
      {observation.recordingHealthReason ? (
        <TargetDetail
          label={messages.recordingHealthReasonLabel}
          value={observation.recordingHealthReason}
        />
      ) : null}
      <TargetDetail
        label={messages.durabilityConfirmationLabel}
        value={
          <DurabilityConfirmationState
            label={messages.durabilityConfirmationLabel}
            state={observation.confirmationState}
          />
        }
      />
    </dl>
  );
}

function TargetDetail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="grid min-w-0 gap-1">
      <Label as="dt">{label}</Label>
      <Text as="dd" className="m-0 min-w-0 [overflow-wrap:anywhere]">
        {value}
      </Text>
    </div>
  );
}

function providerSessionOrigin(
  messages: WorkerSessionTimelineMessages,
  observation: WorkerSessionObservation,
): string {
  const providerSession = observation.providerSession;
  if (
    !providerSession?.provider ||
    !providerSession.kind ||
    !providerSession.id
  ) {
    return messages.providerSessionUnavailable;
  }

  return `${providerSession.provider} / ${providerSession.kind} / ${providerSession.id}`;
}

function workOrigin(
  messages: WorkerSessionTimelineMessages,
  observation: WorkerSessionObservation,
): string {
  return (
    firstNonEmpty(
      observation.workName,
      observation.workId,
      observation.workIds[0],
    ) ?? messages.unknownValueLabel
  );
}

function recordingHealthValue(
  messages: WorkerSessionTimelineMessages,
  observation: WorkerSessionObservation,
): string {
  return observation.recordingHealth
    ? messages.recordingHealthLabel(observation.recordingHealth)
    : messages.recordingHealthPending;
}

function firstNonEmpty(
  ...values: Array<string | null | undefined>
): string | undefined {
  return (
    values.find(
      (value) =>
        value !== undefined && value !== null && value.trim().length > 0,
    ) ?? undefined
  );
}
