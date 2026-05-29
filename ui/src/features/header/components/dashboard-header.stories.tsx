import "../../../styles.css";

import { DashboardHeader } from "./dashboard-header";
import {
  historicalWorkOutcomeSnapshot,
  liveWorkOutcomeSnapshot,
} from "../../../stories/dashboardStorySupport";

export default {
  title: "you-agent-factory/Dashboard/Dashboard Header",
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

const namedFactorySessionSummary = {
  factoryDir: "/workspace/customer-factory",
  folderPath: "/workspace/customer-factory",
  id: "customer-factory::dashboard",
  isDefault: false,
  project: "customer-factory",
  target: {
    kind: "named" as const,
    name: "dashboard",
  },
};

const analyticsFactorySessionSummary = {
  factoryDir: "/workspace/analytics-suite",
  folderPath: "/workspace/analytics-suite",
  id: "analytics-suite::planner",
  isDefault: false,
  project: "analytics-suite",
  target: {
    kind: "named" as const,
    name: "planner",
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
              sessions: [
                defaultFactorySessionSummary,
                namedFactorySessionSummary,
                analyticsFactorySessionSummary,
              ],
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
    <div style={{ margin: "0 auto", maxWidth: "1280px", width: "100%" }}>
      <DashboardHeader />
    </div>
  ),
};
