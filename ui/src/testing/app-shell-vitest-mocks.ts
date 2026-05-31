import { vi } from "vitest";
import type { DashboardTopology } from "../api/dashboard";
import { buildDashboardTestGraphLayout } from "./app-shell-test-graph-layout";

vi.mock("../features/flowchart/lib/layout", async () => {
  const actual = await vi.importActual<
    typeof import("../features/flowchart/lib/layout")
  >("../features/flowchart/lib/layout");

  return {
    ...actual,
    buildGraphLayout: async (topology: DashboardTopology) =>
      buildDashboardTestGraphLayout(topology),
  };
});

vi.mock(
  "../features/current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual<
      typeof import("../features/current-factory-definition/hooks/useCurrentFactoryDefinition")
    >("../features/current-factory-definition/hooks/useCurrentFactoryDefinition");

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);
