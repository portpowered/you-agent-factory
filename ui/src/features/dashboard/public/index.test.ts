import * as dashboardPublic from "./index";
import { DashboardScreen } from "../components/dashboard-screen";

describe("dashboard public barrel", () => {
  it("exports DashboardScreen as the app composition entrypoint", () => {
    expect(dashboardPublic.DashboardScreen).toBe(DashboardScreen);
  });
});
