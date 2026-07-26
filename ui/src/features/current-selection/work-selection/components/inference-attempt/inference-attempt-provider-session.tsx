import type { DashboardInferenceAttempt } from "../../../../../api/dashboard/types";
import { Code, Text } from "@you-agent-factory/components/primitives";
import { getProviderSessionLogTarget } from "../../../../../components/ui/formatters";
import {
  getLoadableProviderSessionRef,
  type LoadableProviderSessionRef,
  providerSessionSelectionKey,
} from "../../../../provider-session-detail/lib/provider-session-ref";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionWorkstationDetailMessages,
} from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionSelectableButton } from "../../../base/components/presentation/current-selection-selectable-button";
import { CurrentSelectionLabel } from "../../../base/components/presentation/current-selection-label";
import { CurrentSelectionSupportingText } from "../../../base/components/presentation/current-selection-supporting-text";
import { InferenceAttemptDetail } from "./inference-attempt-detail";

export interface InferenceAttemptProviderSessionDetailsProps {
  attempt: DashboardInferenceAttempt;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  selectedProviderSessionKey?: string | null;
}

export interface InferenceAttemptProviderSessionPreviewProps
  extends InferenceAttemptProviderSessionDetailsProps {}

export function InferenceAttemptProviderSessionDetails({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: InferenceAttemptProviderSessionDetailsProps) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const workstationMessages = useCurrentSelectionWorkstationDetailMessages();
  const state = useInferenceAttemptProviderSessionState({
    attempt,
    selectedProviderSessionKey,
  });

  if (!state.providerSessionLabel) {
    return (
      <InferenceAttemptDetail
        code={!state.providerSessionLogTarget}
        label={detailMessages.providerSessionLabel}
        value={state.providerSessionLabel}
      />
    );
  }

  if (state.loadableProviderSession && onSelectProviderSession) {
    const loadableProviderSession = state.loadableProviderSession;

    return (
      <div className="grid gap-1">
        <CurrentSelectionLabel>
          {detailMessages.providerSessionLabel}
        </CurrentSelectionLabel>
        <CurrentSelectionSelectableButton
          aria-label={workstationMessages.selectProviderSessionLabel(
            state.providerSessionLabel,
            attempt.dispatch_id,
          )}
          onClick={() => onSelectProviderSession(loadableProviderSession)}
          selected={state.providerSessionSelected}
          variant="card"
        >
          <Text as="span" variant="supporting">
            {state.providerSessionSelected
              ? workstationMessages.providerSessionSelectedAction
              : workstationMessages.providerSessionSelectAction}
          </Text>
          <Code>{state.providerSessionLabel}</Code>
        </CurrentSelectionSelectableButton>
      </div>
    );
  }

  return (
    <div className="grid gap-1">
      <CurrentSelectionLabel>
        {detailMessages.providerSessionLabel}
      </CurrentSelectionLabel>
      <Code>{state.providerSessionLabel}</Code>
      <CurrentSelectionSupportingText tone="status">
        {workstationMessages.providerSessionSelectionUnavailable}
      </CurrentSelectionSupportingText>
    </div>
  );
}

export function InferenceAttemptProviderSessionPreview({
  attempt,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: InferenceAttemptProviderSessionPreviewProps) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const workstationMessages = useCurrentSelectionWorkstationDetailMessages();
  const state = useInferenceAttemptProviderSessionState({
    attempt,
    selectedProviderSessionKey,
  });

  if (!state.providerSessionLabel) {
    return null;
  }

  if (state.loadableProviderSession && onSelectProviderSession) {
    const loadableProviderSession = state.loadableProviderSession;

    return (
      <CurrentSelectionSelectableButton
        aria-label={workstationMessages.selectProviderSessionLabel(
          state.providerSessionLabel,
          attempt.dispatch_id,
        )}
        onClick={() => onSelectProviderSession(loadableProviderSession)}
        selected={state.providerSessionSelected}
      >
        {state.providerSessionSelected
          ? workstationMessages.providerSessionSelectedAction
          : detailMessages.providerSessionLabel}
      </CurrentSelectionSelectableButton>
    );
  }

  return (
    <CurrentSelectionSupportingText tone="status">
      {workstationMessages.providerSessionSelectionUnavailable}
    </CurrentSelectionSupportingText>
  );
}

function useInferenceAttemptProviderSessionState({
  attempt,
  selectedProviderSessionKey,
}: {
  attempt: DashboardInferenceAttempt;
  selectedProviderSessionKey?: string | null;
}) {
  const workstationMessages = useCurrentSelectionWorkstationDetailMessages();
  const providerSessionLogTarget = getProviderSessionLogTarget(
    attempt.provider_session,
    attempt.request_time,
  );
  const loadableProviderSession = getLoadableProviderSessionRef({
    dispatch_id: attempt.dispatch_id,
    provider_session: attempt.provider_session,
  });
  const providerSessionLabel = attempt.provider_session
    ? formatLocalizedProviderSessionLabel(
        attempt.provider_session,
        workstationMessages,
      )
    : undefined;
  const providerSessionSelected =
    loadableProviderSession !== null &&
    selectedProviderSessionKey ===
      providerSessionSelectionKey(loadableProviderSession);

  return {
    loadableProviderSession,
    providerSessionLabel,
    providerSessionLogTarget,
    providerSessionSelected,
  };
}

function formatLocalizedProviderSessionLabel(
  session: DashboardInferenceAttempt["provider_session"],
  workstationMessages: ReturnType<
    typeof useCurrentSelectionWorkstationDetailMessages
  >,
): string {
  if (!session?.id) {
    return workstationMessages.unavailableValue;
  }

  const localizedKind = localizeProviderSessionKind(
    session.kind,
    workstationMessages,
  );
  const parts = [session.provider, localizedKind].filter(
    (value): value is string => value !== undefined && value !== "",
  );

  if (parts.length === 0) {
    return session.id;
  }

  return `${parts.join(" / ")} / ${session.id}`;
}

function localizeProviderSessionKind(
  kind: string | undefined,
  workstationMessages: ReturnType<
    typeof useCurrentSelectionWorkstationDetailMessages
  >,
): string | undefined {
  const normalizedKind = kind?.trim();
  if (!normalizedKind) {
    return undefined;
  }

  return workstationMessages.localizeProviderSessionKind(normalizedKind);
}
