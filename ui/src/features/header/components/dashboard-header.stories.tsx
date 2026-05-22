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

const defaultFactorySessionSummary = {
  factoryDir: "/workspace/root",
  folderPath: "/workspace/root",
  id: "~default",
  isDefault: true,
  project: "root",
  target: {
    kind: "default" as const,
  },
};

export const ResponsiveVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: {
            body: {
              sessions: [defaultFactorySessionSummary],
            },
          },
        },
      ],
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
