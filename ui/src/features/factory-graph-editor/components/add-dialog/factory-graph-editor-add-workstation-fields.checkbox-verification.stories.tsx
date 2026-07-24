import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../../../styles.css";
import { expectStyledCheckboxInStory } from "../../../../testing/checkbox-story-helpers";
import { createEmptyEditableWorkstationCronDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import { getWorkstationDetailMessages } from "../../../current-selection/workstation-selection/messages/workstation-detail";
import type { FactoryGraphAddEntityDraft } from "../../lib/editor/factory-graph-editor-additions";
import { getFactoryGraphEditorMessages } from "../../messages/editor";
import { FactoryGraphEditorAddWorkstationFields } from "./factory-graph-editor-add-workstation-fields";

const messages = getFactoryGraphEditorMessages();
const workstationMessages = getWorkstationDetailMessages();

function buildCronWorkstationDraft(): Extract<
  FactoryGraphAddEntityDraft,
  { kind: "workstation" }
> {
  return {
    behavior: "CRON",
    body: "",
    cron: createEmptyEditableWorkstationCronDraft(),
    kind: "workstation",
    name: "scheduler",
    workerName: "writer",
    workstationType: "MODEL_WORKSTATION",
  };
}

function FactoryGraphCronCheckboxVerificationStory() {
  const [draft, setDraft] = useState(buildCronWorkstationDraft);

  return (
    <FactoryGraphEditorAddWorkstationFields
      currentFactoryDefinition={null}
      draft={draft}
      errors={{}}
      messages={messages}
      onChange={setDraft}
    />
  );
}

export default {
  title: "you-agent-factory/Checkbox Consistency/Factory Graph Editor",
  tags: ["test"],
};

export const CronTriggerAtStart = {
  render: () => <FactoryGraphCronCheckboxVerificationStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const checkbox = await canvas.findByRole("checkbox", {
      name: workstationMessages.cronTriggerAtStartFieldLabel,
    });

    expectStyledCheckboxInStory(checkbox);
    expect(checkbox).not.toBeChecked();

    await userEvent.click(
      canvas.getByText(workstationMessages.cronTriggerAtStartFieldLabel),
    );
    expect(checkbox).toBeChecked();

    checkbox.focus();
    await userEvent.keyboard(" ");
    expect(checkbox).not.toBeChecked();
  },
};
