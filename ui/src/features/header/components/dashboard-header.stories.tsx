import "../../../styles.css";

import { DashboardHeader } from "./dashboard-header";
import {
  historicalWorkOutcomeSnapshot,
  liveWorkOutcomeSnapshot,
} from "../../../stories/dashboardStorySupport";

export default {
  title: "Infinite You/Dashboard/Dashboard Header",
  component: DashboardHeader,
  tags: ["test"],
};

export const ResponsiveVerification = {
  parameters: {
    dashboardApi: {
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <DashboardHeader />
    </div>
  ),
};
