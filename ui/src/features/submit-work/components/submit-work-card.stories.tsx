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
    submitWorkTypes: [{ work_type_name: "story" }, { work_type_name: "task" }],
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });
    const scope = within(card);

    await expect(
      scope.queryByText(
        "Send a new request to the current factory from the dashboard.",
      ),
    ).toBeNull();
    const workType = scope.getByRole("combobox", { name: "Work type" });
    const requestName = scope.getByRole("textbox", { name: "Request name" });
    const requestText = scope.getByRole("textbox", { name: "Text item 1" });
    const submitButton = scope.getByRole("button", { name: "Submit work" });

    await expect(submitButton).toBeDisabled();
    await userEvent.selectOptions(workType, "story");
    await userEvent.type(requestName, "Driver review");
    await userEvent.type(
      requestText,
      "Review the queue and summarize the next driver issue.",
    );
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

    await expect(
      scope.getByRole("combobox", { name: "Work type" }),
    ).toBeDisabled();
    await expect(
      scope.getByRole("textbox", { name: "Request name" }),
    ).toBeDisabled();
    await expect(
      scope.getByRole("textbox", { name: "Text item 1" }),
    ).toBeDisabled();
    await expect(
      scope.getByRole("button", { name: "Submit work" }),
    ).toBeDisabled();
    await expect(
      scope.getByText("No work types are available to submit right now."),
    ).toBeVisible();
  },
};

export const FailureRetry = {
  render: () => (
    <SubmitWorkCard
      draft={{
        items: [{ id: "submission-item-1", text: "Retry the broken submission.", type: "text" }],
        requestName: "Retry dashboard request",
        workTypeName: "story",
      }}
      onAddItem={() => {}}
      onItemTextChange={() => {}}
      onRemoveItem={() => {}}
      onRequestNameChange={() => {}}
      onStageFileItems={() => {}}
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

    await expect(
      scope.getByRole("combobox", { name: "Work type" }),
    ).toHaveValue("story");
    await expect(
      scope.getByRole("textbox", { name: "Request name" }),
    ).toHaveValue("Retry dashboard request");
    await expect(scope.getByRole("textbox", { name: "Text item 1" })).toHaveValue(
      "Retry the broken submission.",
    );
    await expect(scope.getByText("work_type_name is required")).toBeVisible();
    await expect(
      scope.getByRole("button", { name: "Submit work" }),
    ).toBeEnabled();
  },
};

export const LocalizedZhCN = {
  args: {
    locale: "zh-CN",
    submitWorkTypes: [{ work_type_name: "story" }, { work_type_name: "task" }],
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "提交工作" });
    const scope = within(card);

    await expect(
      scope.getByRole("combobox", { name: "工作类型" }),
    ).toBeVisible();
    await expect(
      scope.getByRole("textbox", { name: "请求名称" }),
    ).toBeVisible();
    await expect(scope.getByRole("list", { name: "提交项" })).toBeVisible();
    await expect(scope.getByRole("textbox", { name: "文本项 1" })).toBeVisible();
    await expect(
      scope.getByRole("button", { name: "提交工作" }),
    ).toBeDisabled();
  },
};

function submitButton(card: HTMLElement): HTMLElement {
  const button = card.querySelector<HTMLElement>('button[type="submit"]');

  if (!(button instanceof HTMLElement)) {
    throw new Error("expected submit-work submit button");
  }

  return button;
}

export const SharedWorkContentRowChrome = {
  render: () => (
    <SubmitWorkCard
      draft={{
        items: [
          { id: "submission-item-1", text: "Review the active queue.", type: "text" },
          { id: "submission-item-2", stagingStatus: "idle", type: "image" },
        ],
        requestName: "Multimodal review",
        workTypeName: "story",
      }}
      onAddItem={() => {}}
      onItemTextChange={() => {}}
      onRemoveItem={() => {}}
      onRequestNameChange={() => {}}
      onStageFileItems={() => {}}
      onSubmit={() => {}}
      onWorkTypeNameChange={() => {}}
      status={{
        kind: "guidance",
        message: "Shared work-content row chrome for text and image items.",
      }}
      submitWorkTypeNames={["story", "task"]}
      widgetId="submit-work-shared-row-chrome"
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });
    const scope = within(card);
    const submissionItems = scope.getByRole("list", { name: "Submission items" });
    const rows = within(submissionItems).getAllByRole("listitem");

    await expect(rows).toHaveLength(2);
    await expect(scope.getByText("Text")).toBeVisible();
    await expect(scope.getByText("Image")).toBeVisible();
    for (const row of rows) {
      await expect(row).toHaveClass("border-af-border");
      await expect(row).toHaveClass("bg-af-panel");
    }
    await expect(
      scope.getByRole("button", { name: "Remove text item 1" }),
    ).toBeVisible();
    await expect(
      scope.getByRole("button", { name: "Remove image item 2" }),
    ).toBeVisible();
  },
};

export const StableActionAlignment = {
  render: () => (
    <div className="grid gap-4">
      <div className="w-full max-w-xs">
        <SubmitWorkCard
          draft={{
            items: [{ id: "submission-item-1", text: "", type: "text" }],
            requestName: "Driver review",
            workTypeName: "story",
          }}
          onAddItem={() => {}}
          onItemTextChange={() => {}}
          onRemoveItem={() => {}}
          onRequestNameChange={() => {}}
          onStageFileItems={() => {}}
          onSubmit={() => {}}
          onWorkTypeNameChange={() => {}}
          status={{
            kind: "guidance",
            message:
              "Ready to submit with a long guidance message that wraps on narrow widths without moving the primary action.",
          }}
          submitWorkTypeNames={["story", "task"]}
          widgetId="submit-work-ready"
        />
      </div>
      <div className="w-full max-w-xs">
        <SubmitWorkCard
          draft={{
            items: [{ id: "submission-item-1", text: "", type: "text" }],
            requestName: "Driver review",
            workTypeName: "story",
          }}
          isSubmitting
          onAddItem={() => {}}
          onItemTextChange={() => {}}
          onRemoveItem={() => {}}
          onRequestNameChange={() => {}}
          onStageFileItems={() => {}}
          onSubmit={() => {}}
          onWorkTypeNameChange={() => {}}
          status={{
            kind: "submitting",
            message:
              "Sending your request while the status text remains wrapped and the button stays anchored at the same right edge.",
          }}
          submitWorkTypeNames={["story", "task"]}
          widgetId="submit-work-submitting"
        />
      </div>
      <div className="w-full max-w-xs">
        <SubmitWorkCard
          draft={{
            items: [{ id: "submission-item-1", text: "", type: "text" }],
            requestName: "",
            workTypeName: "story",
          }}
          onAddItem={() => {}}
          onItemTextChange={() => {}}
          onRemoveItem={() => {}}
          onRequestNameChange={() => {}}
          onStageFileItems={() => {}}
          onSubmit={() => {}}
          onWorkTypeNameChange={() => {}}
          status={{
            kind: "success",
            message:
              "Your request was submitted. Trace ID: trace-submit-story-with-extra-copy-to-force-wrapping.",
          }}
          submitWorkTypeNames={["story", "task"]}
          widgetId="submit-work-success"
        />
      </div>
      <div className="w-full max-w-xs">
        <SubmitWorkCard
          draft={{
            items: [{ id: "submission-item-1", text: "Retry the broken submission.", type: "text" }],
            requestName: "Retry dashboard request",
            workTypeName: "story",
          }}
          onAddItem={() => {}}
          onItemTextChange={() => {}}
          onRemoveItem={() => {}}
          onRequestNameChange={() => {}}
          onStageFileItems={() => {}}
          onSubmit={() => {}}
          onWorkTypeNameChange={() => {}}
          status={{
            kind: "error",
            message:
              "The server rejected this submission with a long retryable error that should wrap without shifting the button.",
          }}
          submitWorkTypeNames={["story", "task"]}
          widgetId="submit-work-error"
        />
      </div>
      <div className="w-full max-w-xs">
        <SubmitWorkCard
          draft={{
            items: [{ id: "submission-item-1", text: "", type: "text" }],
            requestName: "",
            workTypeName: "",
          }}
          onAddItem={() => {}}
          onItemTextChange={() => {}}
          onRemoveItem={() => {}}
          onRequestNameChange={() => {}}
          onStageFileItems={() => {}}
          onSubmit={() => {}}
          onWorkTypeNameChange={() => {}}
          status={{
            kind: "validation-error",
            message:
              "Choose a work type and enter a request name before submitting so the wrapped validation summary still leaves the action pinned right.",
          }}
          submitWorkTypeNames={["story", "task"]}
          validationErrors={{
            requestName: "Enter a request name before submitting.",
            workTypeName: "Choose a work type before submitting.",
          }}
          widgetId="submit-work-validation"
        />
      </div>
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const cards = await canvas.findAllByRole("article", { name: "Submit work" });
    const buttonRights: number[] = [];

    for (const card of cards) {
      buttonRights.push(submitButton(card).getBoundingClientRect().right);
    }

    const referenceRight = buttonRights[0];
    if (referenceRight === undefined) {
      throw new Error("expected submit-work button alignment reference");
    }

    for (const rightEdge of buttonRights) {
      expect(Math.abs(rightEdge - referenceRight)).toBeLessThanOrEqual(1);
    }

    await expect(
      canvas.getByText(
        "Choose a work type and enter a request name before submitting so the wrapped validation summary still leaves the action pinned right.",
      ),
    ).toBeVisible();
  },
};
