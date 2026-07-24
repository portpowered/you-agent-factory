import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { useCurrentActivityGraphCardViewModel } from "../hooks/use-current-activity-graph-card-view-model";
import { useCurrentActivityGraphState } from "../hooks/use-current-activity-graph-state";
import {
  type ReactFlowCurrentActivityCardProps,
  ReactFlowCurrentActivityCardView,
} from "./react-flow-current-activity-card-view";

export function ReactFlowCurrentActivityCard(
  props: ReactFlowCurrentActivityCardProps,
) {
  const { sessionID } = useDashboardSession();
  const activityGraphState = useCurrentActivityGraphState(
    props.snapshot,
    props.locale,
    sessionID,
  );
  const viewModel = useCurrentActivityGraphCardViewModel({
    ...props,
    graphController: activityGraphState,
  });

  return (
    <ReactFlowCurrentActivityCardView
      {...props}
      sessionID={sessionID}
      viewModel={viewModel}
      showHeaderActions
    />
  );
}
