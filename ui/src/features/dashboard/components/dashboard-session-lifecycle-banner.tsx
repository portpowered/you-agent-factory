import type { DashboardSessionBracket } from "../../../api/dashboard/types";
import type { DashboardStreamState } from "../../../api/dashboard/types";
import { AlertPanel, DashboardLabel, DashboardText } from "../../../components/ui";
import { getDashboardSessionLifecycleMessages } from "../messages/dashboard-session-lifecycle";

export interface DashboardSessionLifecycleBannerProps {
  bracket?: DashboardSessionBracket;
  factoryState?: string;
  locale?: string | null;
  phase?: string;
  streamState: DashboardStreamState;
}

export function DashboardSessionLifecycleBanner({
  bracket,
  factoryState,
  locale,
  phase,
  streamState,
}: DashboardSessionLifecycleBannerProps) {
  const messages = getDashboardSessionLifecycleMessages(locale);
  const streamNotice = streamNoticeForState(streamState, messages);
  const lifecycleNotice = lifecycleNoticeForBracket(
    bracket,
    factoryState,
    phase,
    messages,
  );

  if (!streamNotice && !lifecycleNotice) {
    return null;
  }

  return (
    <section
      aria-label={messages.sessionStartedLabel}
      aria-live="polite"
      className="grid gap-2"
      data-testid="dashboard-session-lifecycle-banner"
    >
      {streamNotice ? (
        <AlertPanel tone={streamNotice.tone}>{streamNotice.message}</AlertPanel>
      ) : null}
      {lifecycleNotice ? (
        <div className="grid gap-2 rounded-md border border-outline p-3 sm:grid-cols-2">
          <LifecycleMetric
            label={lifecycleNotice.title}
            value={lifecycleNotice.summary}
          />
          {lifecycleNotice.resultStatus ? (
            <LifecycleMetric
              label={messages.resultStatusLabel}
              value={lifecycleNotice.resultStatus}
            />
          ) : null}
          {phase ? (
            <LifecycleMetric label={messages.phaseLabel} value={phase} />
          ) : null}
          {lifecycleNotice.artifactRefs ? (
            <LifecycleMetric
              label={messages.artifactRefsLabel}
              value={lifecycleNotice.artifactRefs}
            />
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function LifecycleMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1">
      <DashboardLabel>{label}</DashboardLabel>
      <DashboardText>{value}</DashboardText>
    </div>
  );
}

function streamNoticeForState(
  streamState: DashboardStreamState,
  messages: ReturnType<typeof getDashboardSessionLifecycleMessages>,
): { message: string; tone: "info" | "warning" } | null {
  if (streamState.status === "reconnecting") {
    return {
      message: messages.reconnectingStreamLabel,
      tone: "info",
    };
  }
  if (streamState.status === "offline") {
    return {
      message: streamState.message || messages.staleStreamLabel,
      tone: "warning",
    };
  }
  return null;
}

function lifecycleNoticeForBracket(
  bracket: DashboardSessionBracket | undefined,
  factoryState: string | undefined,
  phase: string | undefined,
  messages: ReturnType<typeof getDashboardSessionLifecycleMessages>,
): {
  artifactRefs?: string;
  resultStatus?: string;
  summary: string;
  title: string;
} | null {
  if (!bracket && !phase && !factoryState) {
    return null;
  }
  if (!bracket) {
    if (factoryState === "PAUSED") {
      return {
        resultStatus: factoryState,
        summary: messages.sessionPausedLabel,
        title: messages.lifecycleControlStatusLabel,
      };
    }
    return {
      summary: phase ?? "",
      title: messages.phaseLabel,
    };
  }

  const artifactRefs =
    bracket.artifact_ids && bracket.artifact_ids.length > 0
      ? bracket.artifact_ids.join(", ")
      : undefined;

  if (bracket.terminal) {
    if (bracket.failure_reason || bracket.failure_message) {
      return {
        artifactRefs,
        resultStatus: bracket.result_status,
        summary:
          bracket.failure_message ??
          bracket.failure_reason ??
          messages.failureLabel,
        title: messages.failureLabel,
      };
    }
    return {
      artifactRefs,
      resultStatus: bracket.result_status,
      summary: bracket.final_status ?? messages.terminalSuccessLabel,
      title: messages.completedLabel,
    };
  }

  if (bracket.result_status === "PARTIAL" || bracket.result_status === "FAILED_WITH_PARTIAL") {
    return {
      artifactRefs,
      resultStatus: bracket.result_status,
      summary:
        bracket.result_summary?.map((part) => part.text).filter(Boolean).join(" ") ||
        messages.partialResultLabel,
      title: messages.partialResultLabel,
    };
  }

  const reflectedLifecycleStatus =
    bracket.lifecycle_control_status ??
    (factoryState === "PAUSED" ? "PAUSED" : undefined);

  if (reflectedLifecycleStatus === "PAUSED") {
    return {
      resultStatus: reflectedLifecycleStatus,
      summary: bracket.paused_at ?? messages.sessionPausedLabel,
      title: messages.sessionPausedLabel,
    };
  }

  if (reflectedLifecycleStatus === "RUNNING") {
    return {
      resultStatus: reflectedLifecycleStatus,
      summary: bracket.resumed_at ?? messages.sessionRunningLabel,
      title: messages.sessionRunningLabel,
    };
  }

  return {
    artifactRefs,
    resultStatus: bracket.result_status,
    summary: bracket.started_at ?? messages.sessionStartedLabel,
    title: messages.sessionStartedLabel,
  };
}
