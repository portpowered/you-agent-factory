import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../../dashboard/state/dashboardSessionStore";
import { SubmitWorkWidget } from "./index";

describe("submit-work public barrel", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  it("keeps SubmitWorkWidget rendering the submit form through the public export", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const card = screen.getByRole("article", { name: "Submit work" });

    expect(
      within(card).getByRole("combobox", { name: "Work type" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("textbox", { name: "Request name" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("list", { name: "Submission items" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("textbox", { name: "Text item 1" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("button", { name: "Submit work" }),
    ).toBeTruthy();
  });
});

function renderSubmitWorkWidget(element: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{element}</QueryClientProvider>,
  );
}
