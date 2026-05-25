import { screen, within } from "@testing-library/react";

import {
  getSubmitWorkCard,
  getSubmitWorkCardControls,
  submitWorkCardQueryContract,
} from "./submit-work-card-queries";

export const activeWorkLabel = "Active Story";

export function submitWorkCardControls() {
  const dashboardGrid = screen.getByRole("region", {
    name: submitWorkCardQueryContract.dashboardRegionName,
  });
  const submitWorkCard = getSubmitWorkCard(within(dashboardGrid));
  const submitWorkScope = within(submitWorkCard);

  return {
    ...getSubmitWorkCardControls(submitWorkScope),
    submitWorkScope,
  };
}
