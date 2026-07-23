import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { expectStyledCheckbox } from "../../../../testing/checkbox-test-helpers";
import { createEmptyEditableWorkstationCronDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import { getWorkstationDetailMessages } from "../../../current-selection/workstation-selection/messages/workstation-detail";
import { getFactoryGraphEditorMessages } from "../../messages/editor";
import { FactoryGraphEditorAddWorkstationFields } from "./factory-graph-editor-add-workstation-fields";

let restoreBrowserShims: (() => void) | undefined;

const messages = getFactoryGraphEditorMessages();
const workstationMessages = getWorkstationDetailMessages();

function buildCronWorkstationDraft() {
  return {
    behavior: "CRON" as const,
    body: "",
    cron: createEmptyEditableWorkstationCronDraft(),
    kind: "workstation" as const,
    name: "scheduler",
    workerName: "writer",
    workstationType: "MODEL_WORKSTATION" as const,
  };
}

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("FactoryGraphEditorAddWorkstationFields cron checkbox", () => {
  it("renders the shared styled checkbox for cron trigger-at-start", () => {
    render(
      <FactoryGraphEditorAddWorkstationFields
        currentFactoryDefinition={null}
        draft={buildCronWorkstationDraft()}
        errors={{}}
        messages={messages}
        onChange={vi.fn()}
      />,
    );

    const triggerAtStartCheckbox = screen.getByRole("checkbox", {
      name: workstationMessages.cronTriggerAtStartFieldLabel,
    });

    expectStyledCheckbox(triggerAtStartCheckbox);
    expect(triggerAtStartCheckbox.checked).toBe(false);
  });

  it("toggles cron trigger-at-start from label clicks and Space while focused", async () => {
    const user = userEvent.setup();
    let draft = buildCronWorkstationDraft();
    const onChange = vi.fn((nextDraft) => {
      draft = nextDraft;
    });

    const { rerender } = render(
      <FactoryGraphEditorAddWorkstationFields
        currentFactoryDefinition={null}
        draft={draft}
        errors={{}}
        messages={messages}
        onChange={onChange}
      />,
    );

    const triggerAtStartCheckbox = screen.getByRole("checkbox", {
      name: workstationMessages.cronTriggerAtStartFieldLabel,
    });

    await user.click(
      screen.getByText(workstationMessages.cronTriggerAtStartFieldLabel),
    );

    rerender(
      <FactoryGraphEditorAddWorkstationFields
        currentFactoryDefinition={null}
        draft={draft}
        errors={{}}
        messages={messages}
        onChange={onChange}
      />,
    );
    expect(triggerAtStartCheckbox.checked).toBe(true);

    triggerAtStartCheckbox.focus();
    await user.keyboard(" ");

    rerender(
      <FactoryGraphEditorAddWorkstationFields
        currentFactoryDefinition={null}
        draft={draft}
        errors={{}}
        messages={messages}
        onChange={onChange}
      />,
    );
    expect(triggerAtStartCheckbox.checked).toBe(false);
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        cron: expect.objectContaining({ triggerAtStart: false }),
      }),
    );
  });
});
