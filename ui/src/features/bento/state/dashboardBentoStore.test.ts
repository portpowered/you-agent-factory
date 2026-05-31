import { useDashboardBentoStore } from "./dashboardBentoStore";

describe("useDashboardBentoStore refreshToken", () => {
  beforeEach(() => {
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
  });

  it("starts at zero and only advances when incrementRefreshToken runs", () => {
    expect(useDashboardBentoStore.getState().refreshToken).toBe(0);

    useDashboardBentoStore.getState().incrementRefreshToken();

    expect(useDashboardBentoStore.getState().refreshToken).toBe(1);
  });
});
