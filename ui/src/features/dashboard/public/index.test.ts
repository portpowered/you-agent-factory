import { DashboardScreen } from "../components/dashboard-screen";
import * as dashboardPublic from "./index";

describe("dashboard public barrel", () => {
  it("exports DashboardScreen as the app composition entrypoint", () => {
    expect(dashboardPublic.DashboardScreen).toBe(DashboardScreen);
  });
});
