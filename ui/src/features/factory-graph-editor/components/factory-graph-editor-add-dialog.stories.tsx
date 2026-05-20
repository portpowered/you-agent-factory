import { expect, within } from "storybook/test";

import "../../../styles.css";
import { FactoryGraphEditorAddEntityDialog } from "./factory-graph-editor-add-dialog";
import type { CanonicalFactoryDefinition } from "../factory-graph-draft-types";

const CURRENT_FACTORY_DEFINITION: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
      ],
    },
  ],
  workstations: [],
};

export default {
  title: "Agent Factory/Dashboard/Factory Graph Editor Add Dialog",
  tags: ["test"],
};

export const AddWorkType = {
  render: () => (
    <FactoryGraphEditorAddEntityDialog
      currentFactoryDefinition={CURRENT_FACTORY_DEFINITION}
      draft={{
        initialStateName: "",
        kind: "work-type",
        name: "",
      }}
      errors={{}}
      isOpen={true}
      onChange={() => {}}
      onClose={() => {}}
      onSubmit={() => {}}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      {
        name: "Add work type",
      },
    );

    await expect(
      within(dialog).getByText(
        "Define a new work type and its first ordered state.",
      ),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("textbox", { name: "Identifier" }),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("textbox", { name: "First state" }),
    ).toBeVisible();
    await expect(
      within(dialog).getByText(
        "New work types start with one required ordered state.",
      ),
    ).toBeVisible();
  },
};

export const AddWorkState = {
  render: () => (
    <FactoryGraphEditorAddEntityDialog
      currentFactoryDefinition={CURRENT_FACTORY_DEFINITION}
      draft={{
        kind: "work-state",
        name: "",
        stateType: "PROCESSING",
        workTypeName: "story",
      }}
      errors={{}}
      isOpen={true}
      onChange={() => {}}
      onClose={() => {}}
      onSubmit={() => {}}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      {
        name: "Add work state",
      },
    );

    await expect(
      within(dialog).getByText(
        "Append a new ordered state to an existing work type.",
      ),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("textbox", { name: "Identifier" }),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("combobox", { name: "Work type" }),
    ).toHaveValue("story");
    await expect(
      within(dialog).getByRole("combobox", { name: "State type" }),
    ).toHaveValue("PROCESSING");
  },
};

export const WorkStateValidation = {
  render: () => (
    <FactoryGraphEditorAddEntityDialog
      currentFactoryDefinition={CURRENT_FACTORY_DEFINITION}
      draft={{
        kind: "work-state",
        name: "queued",
        stateType: "PROCESSING",
        workTypeName: "",
      }}
      errors={{
        name: 'Work type "story" already defines a state named "queued".',
        workTypeName: "Choose a work type before adding a work state.",
      }}
      isOpen={true}
      onChange={() => {}}
      onClose={() => {}}
      onSubmit={() => {}}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      {
        name: "Add work state",
      },
    );

    await expect(
      within(dialog).getByText(
        "Choose a work type before adding a work state.",
      ),
    ).toBeVisible();
    await expect(
      within(dialog).getByText(
        'Work type "story" already defines a state named "queued".',
      ),
    ).toBeVisible();
  },
};
