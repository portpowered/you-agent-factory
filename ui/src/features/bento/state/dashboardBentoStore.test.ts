import { useDashboardBentoStore } from "./dashboardBentoStore";

describe("useDashboardBentoStore selectedTraceID", () => {
  beforeEach(() => {
    useDashboardBentoStore.setState({
      refreshToken: 0,
      selectedTraceID: null,
    });
  });

  it("stores and clears the selected trace id", () => {
    useDashboardBentoStore.getState().setSelectedTraceID("trace-123");

    expect(useDashboardBentoStore.getState().selectedTraceID).toBe("trace-123");

    useDashboardBentoStore.getState().resetSelectedTraceID();

    expect(useDashboardBentoStore.getState().selectedTraceID).toBeNull();
  });
});

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
