import { type ReactNode, useState } from "react";
import type { MessageWorkerIdentity } from "../../../api/agent-messages/types";
import { factoryAPIURL } from "../../../api/baseUrl";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { MessageInboxDialog } from "../../agent-messages/components/message-inbox-dialog";
import { MessageThread } from "../../agent-messages/components/message-thread";
import { getAgentMessageMessages } from "../../agent-messages/messages/agent-messages";
import { DashboardWidgetFrame } from "../../bento/components/dashboard-widget-frame/dashboard-widget-frame";
import {
  useWorkerSessionTimelineTarget,
  type WorkerSessionTimelineTargetLoader,
} from "../hooks/useWorkerSessionTimelineTarget";
import { getWorkerSessionTimelineMessages } from "../messages/worker-session-timeline";
import {
  WorkerSessionTimeline,
  type WorkerSessionTimelineProps,
} from "./worker-session-timeline";
import { WorkerSessionTimelineTarget } from "./worker-session-timeline-target";

export interface WorkerSessionTimelineWidgetProps
  extends WorkerSessionTimelineProps {
  headerAction?: ReactNode;
  loadWorkerSessionTargets?: WorkerSessionTimelineTargetLoader;
  locale?: string;
  workID?: string | null;
  widgetId?: string;
}

/** Dashboard chrome for consumers that need a selectable Worker Session card. */
export function WorkerSessionTimelineWidget({
  factorySessionID,
  headerAction,
  loadWorkerSessionTargets,
  locale,
  workID = null,
  widgetId = "worker-session-timeline",
  workerSessionID,
  ...timelineOptions
}: WorkerSessionTimelineWidgetProps) {
  const messages = getWorkerSessionTimelineMessages(locale);
  const targetState = useWorkerSessionTimelineTarget({
    enabled: workerSessionID === null,
    factorySessionID,
    loadTargets: loadWorkerSessionTargets,
    workID,
  });
  const selectedWorkerSessionID =
    workerSessionID ?? targetState.selectedWorkerSessionID;
  const scopeKey = JSON.stringify([
    factoryAPIURL(""),
    factorySessionID,
    selectedWorkerSessionID,
  ]);
  const [inspected, setInspected] = useState<{
    scopeKey: string;
    identity: MessageWorkerIdentity;
  } | null>(null);
  const inspectedIdentity =
    inspected?.scopeKey === scopeKey ? inspected.identity : null;
  const displayedWorkerSessionID =
    inspectedIdentity?.workerSessionId ?? selectedWorkerSessionID;
  const displayedFactorySessionID = inspectedIdentity
    ? (inspectedIdentity.factorySessionId ?? null)
    : factorySessionID;
  const targetIsReady = displayedWorkerSessionID !== null;

  return (
    <DashboardWidgetFrame
      bodyScroll={false}
      headerAction={
        <>
          {headerAction}
          <MessageInboxDialog
            locale={locale}
            onNavigate={(identity) => setInspected({ scopeKey, identity })}
          />
        </>
      }
      title={messages.timelineTitle}
      widgetId={widgetId}
    >
      {workerSessionID === null ? (
        <WorkerSessionTimelineTarget
          messages={messages}
          state={targetState}
          workID={workID}
        />
      ) : null}
      {targetIsReady ? (
        <WorkerSessionTimeline
          {...timelineOptions}
          factorySessionID={displayedFactorySessionID}
          direct={inspectedIdentity?.direct ?? timelineOptions.direct}
          locale={locale}
          showHeading={false}
          workerSessionID={displayedWorkerSessionID}
        />
      ) : null}
      {targetIsReady ? (
        <>
          {inspectedIdentity ? (
            <DashboardActionButton
              type="button"
              onClick={() => setInspected(null)}
            >
              {getAgentMessageMessages(locale).original}
            </DashboardActionButton>
          ) : null}
          <MessageThread
            enabled={timelineOptions.enabled}
            factorySessionID={displayedFactorySessionID}
            workerSessionID={displayedWorkerSessionID}
            locale={locale}
            onNavigate={(identity) => setInspected({ scopeKey, identity })}
          />
        </>
      ) : null}
    </DashboardWidgetFrame>
  );
}
