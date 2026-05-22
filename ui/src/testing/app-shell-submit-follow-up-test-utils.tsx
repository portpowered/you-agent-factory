import { screen, within } from "@testing-library/react";

export const activeWorkLabel = "Active Story";

export function submitWorkCardControls() {
  const dashboardGrid = screen.getByRole("region", {
    name: "you-agent-factory bento board",
  });
  const submitWorkCard = within(dashboardGrid).getByRole("article", {
    name: "Submit work",
  });
  const submitWorkScope = within(submitWorkCard);

  return {
    requestName: submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    }),
    requestText: submitWorkScope.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Request",
    }),
    submitButton: submitWorkScope.getByRole<HTMLButtonElement>("button", {
      name: "Submit work",
    }),
    submitWorkScope,
    workType: submitWorkScope.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    }),
  };
}
