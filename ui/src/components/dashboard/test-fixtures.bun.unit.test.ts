import { describe, expect, it } from "bun:test";
import { twentyNodeDashboardTopology } from "./fixtures/topologies";
import * as dashboardTestFixtures from "./test-fixtures";

describe("dashboard/test-fixtures", () => {
  it("does not re-export the canonical twenty-node topology fixture", () => {
    expect(dashboardTestFixtures).not.toHaveProperty(
      "twentyNodeDashboardTopology",
    );
  });

  it("keeps twentyNodeDashboardSnapshot wired to the canonical topology fixture", () => {
    expect(dashboardTestFixtures.twentyNodeDashboardSnapshot.topology).toBe(
      twentyNodeDashboardTopology,
    );
  });
});
