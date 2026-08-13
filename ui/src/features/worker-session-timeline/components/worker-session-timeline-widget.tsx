import type { ReactNode } from "react";
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
  const targetIsReady =
    workerSessionID !== null || targetState.selectedWorkerSessionID !== null;

  return (
    <DashboardWidgetFrame
      bodyScroll={false}
      headerAction={headerAction}
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
          factorySessionID={factorySessionID}
          locale={locale}
          showHeading={false}
          workerSessionID={selectedWorkerSessionID}
        />
      ) : null}
    </DashboardWidgetFrame>
  );
}
