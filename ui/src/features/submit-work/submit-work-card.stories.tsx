import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { expect, userEvent, within } from "storybook/test";

import { SubmitWorkCard } from "./submit-work-card";
import { SubmitWorkWidget } from "./submit-work-widget";

const withQueryClient = (Story: () => ReactElement) => (
  <QueryClientProvider
    client={
      new QueryClient({
        defaultOptions: {
          mutations: {
            retry: false,
          },
          queries: {
            retry: false,
          },
        },
      })
    }
  >
    <Story />
  </QueryClientProvider>
);

export default {
  title: "Agent Factory/Dashboard/Submit Work Card",
  component: SubmitWorkWidget,
  decorators: [withQueryClient],
};

export const Configured = {
  args: {
    submitWorkTypes: [
      { work_type_name: "story" },
      { work_type_name: "task" },
    ],
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });
    const scope = within(card);

    await expect(
      scope.queryByText("Send a new request to the current factory from the dashboard."),
    ).toBeNull();
    const workType = scope.getByRole("combobox", { name: "Work type" });
    const requestName = scope.getByRole("textbox", { name: "Request name" });
    const requestText = scope.getByRole("textbox", { name: "Request" });
    const submitButton = scope.getByRole("button", { name: "Submit work" });

    await expect(submitButton).toBeDisabled();
    await userEvent.selectOptions(workType, "story");
    await userEvent.type(requestName, "Driver review");
    await userEvent.type(requestText, "Review the queue and summarize the next driver issue.");
    await expect(submitButton).toBeEnabled();
  },
};

export const Unconfigured = {
  args: {
    submitWorkTypes: [],
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });
    const scope = within(card);

    await expect(scope.getByRole("combobox", { name: "Work type" })).toBeDisabled();
    await expect(scope.getByRole("textbox", { name: "Request name" })).toBeDisabled();
    await expect(scope.getByRole("textbox", { name: "Request" })).toBeDisabled();
    await expect(scope.getByRole("button", { name: "Submit work" })).toBeDisabled();
    await expect(scope.getByText("No work types are available to submit right now.")).toBeVisible();
  },
};

export const FailureRetry = {
  render: () => (
    <SubmitWorkCard
      draft={{
        requestName: "Retry dashboard request",
        requestText: "Retry the broken submission.",
        workTypeName: "story",
      }}
      onRequestNameChange={() => {}}
      onRequestTextChange={() => {}}
      onSubmit={() => {}}
      onWorkTypeNameChange={() => {}}
      status={{
        kind: "error",
        message: "work_type_name is required",
      }}
      submitWorkTypeNames={["story", "task"]}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });
    const scope = within(card);

    await expect(scope.getByRole("combobox", { name: "Work type" })).toHaveValue("story");
    await expect(scope.getByRole("textbox", { name: "Request name" })).toHaveValue(
      "Retry dashboard request",
    );
    await expect(scope.getByRole("textbox", { name: "Request" })).toHaveValue(
      "Retry the broken submission.",
    );
    await expect(scope.getByText("work_type_name is required")).toBeVisible();
    await expect(scope.getByRole("button", { name: "Submit work" })).toBeEnabled();
  },
};
