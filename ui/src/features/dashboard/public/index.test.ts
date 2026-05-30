import * as dashboardPublic from "./index";
import { DashboardScreen } from "../components/dashboard-screen";

describe("dashboard public barrel", () => {
  it("exports DashboardScreen as the app composition entrypoint", () => {
    expect(dashboardPublic.DashboardScreen).toBe(DashboardScreen);
  });

  it("does not expose dashboard session scope through the public barrel", () => {
    expect(dashboardPublic).not.toHaveProperty("DashboardSessionProvider");
    expect(dashboardPublic).not.toHaveProperty("useDashboardSession");
  });
});
