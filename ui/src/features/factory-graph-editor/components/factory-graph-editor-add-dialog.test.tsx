import { fireEvent, render, screen } from "@testing-library/react";

import type { CanonicalFactoryDefinition } from "../lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import { FactoryGraphEditorAddEntityDialog } from "./factory-graph-editor-add-dialog";

const currentFactoryDefinition: CanonicalFactoryDefinition = {
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

describe("FactoryGraphEditorAddEntityDialog", () => {
  it("does not render when closed or missing a draft", () => {
    const { rerender } = renderDialog({
      draft: null,
      isOpen: true,
    });

    expect(screen.queryByRole("dialog")).toBeNull();

    rerender(
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={currentFactoryDefinition}
        draft={{ capacity: "1", kind: "resource", name: "gpu" }}
        errors={{}}
        isOpen={false}
        onChange={vi.fn()}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("edits resource capacity fields and submits the form", () => {
    const onChange = vi.fn();
    const onSubmit = vi.fn();
    renderDialog({
      draft: { capacity: "1", kind: "resource", name: "gpu" },
      errors: {
        capacity: "Resource capacity must be a whole number greater than zero.",
      },
      onChange,
      onSubmit,
    });

    const dialog = screen.getByRole("dialog", { name: "Add resource" });
    expect(document.body.contains(dialog)).toBe(true);

    fireEvent.change(screen.getByRole("textbox", { name: "Identifier" }), {
      target: { value: "cpu" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Capacity" }), {
      target: { value: "3" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add entity" }));

    expect(onChange).toHaveBeenNthCalledWith(1, {
      capacity: "1",
      kind: "resource",
      name: "cpu",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      capacity: "3",
      kind: "resource",
      name: "gpu",
    });
    expect(
      screen.getByText(
        "Resource capacity must be a whole number greater than zero.",
      ),
    ).toBeTruthy();
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("renders model worker fields and emits onChange payloads", () => {
    const workerChange = vi.fn();
    renderDialog({
      draft: {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "writer",
        workerType: "MODEL_WORKER",
      },
      errors: {
        modelProvider: "Select a model provider for the new worker.",
      },
      onChange: workerChange,
    });

    expect(screen.getByRole("combobox", { name: "Worker type" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Model provider" })).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Model" })).toBeTruthy();
    expect(screen.queryByRole("textbox", { name: "Command" })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Args" })).toBeNull();

    fireEvent.change(screen.getByRole("combobox", { name: "Model provider" }), {
      target: { value: "CURSOR" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Model" }), {
      target: { value: "gpt-5.5" },
    });

    expect(workerChange).toHaveBeenNthCalledWith(1, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "CURSOR",
      name: "writer",
      workerType: "MODEL_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(2, {
      argsText: "",
      command: "",
      kind: "worker",
      model: "gpt-5.5",
      modelProvider: "",
      name: "writer",
      workerType: "MODEL_WORKER",
    });
    expect(
      screen.getByText("Select a model provider for the new worker."),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Optional. Leave blank to use the provider default model identifier.",
      ),
    ).toBeTruthy();
  });

  it("toggles script worker fields and clears deselected values", () => {
    const workerChange = vi.fn();
    const { rerender } = renderDialog({
      draft: {
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "runner",
        workerType: "MODEL_WORKER",
      },
      errors: {
        modelProvider: "Select a model provider for the new worker.",
      },
      onChange: workerChange,
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Worker type" }), {
      target: { value: "SCRIPT_WORKER" },
    });

    expect(workerChange).toHaveBeenCalledWith({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      workerType: "SCRIPT_WORKER",
    });

    rerender(
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={currentFactoryDefinition}
        draft={{
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "",
          name: "runner",
          workerType: "SCRIPT_WORKER",
        }}
        errors={{}}
        isOpen={true}
        onChange={workerChange}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(screen.queryByRole("combobox", { name: "Model provider" })).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Model" })).toBeNull();
    expect(screen.getByRole("textbox", { name: "Command" })).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Args" })).toBeTruthy();

    fireEvent.change(screen.getByRole("textbox", { name: "Command" }), {
      target: { value: "./run.sh" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Args" }), {
      target: { value: "--verbose\n--dry-run" },
    });

    expect(workerChange).toHaveBeenNthCalledWith(2, {
      argsText: "",
      command: "./run.sh",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      workerType: "SCRIPT_WORKER",
    });
    expect(workerChange).toHaveBeenNthCalledWith(3, {
      argsText: "--verbose\n--dry-run",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      workerType: "SCRIPT_WORKER",
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Worker type" }), {
      target: { value: "MODEL_WORKER" },
    });

    expect(workerChange).toHaveBeenCalledWith({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "runner",
      workerType: "MODEL_WORKER",
    });
  });

  it("renders work-type specific fields", () => {
    const workTypeChange = vi.fn();
    renderDialog({
      draft: { initialStateName: "", kind: "work-type", name: "article" },
      errors: { initialStateName: "Enter the first work-state identifier." },
      onChange: workTypeChange,
    });

    fireEvent.change(screen.getByRole("textbox", { name: "First state" }), {
      target: { value: "drafting" },
    });

    expect(workTypeChange).toHaveBeenCalledWith({
      initialStateName: "drafting",
      kind: "work-type",
      name: "article",
    });
    expect(
      screen.getByText("Enter the first work-state identifier."),
    ).toBeTruthy();
  });

  it("edits work-state selects from the current factory definition", () => {
    const onChange = vi.fn();
    renderDialog({
      draft: {
        kind: "work-state",
        name: "review",
        stateType: "PROCESSING",
        workTypeName: "",
      },
      errors: {
        workTypeName: "Choose a work type before adding a work state.",
      },
      onChange,
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Work type" }), {
      target: { value: "story" },
    });
    fireEvent.change(screen.getByRole("combobox", { name: "State type" }), {
      target: { value: "TERMINAL" },
    });

    expect(onChange).toHaveBeenNthCalledWith(1, {
      kind: "work-state",
      name: "review",
      stateType: "PROCESSING",
      workTypeName: "story",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      kind: "work-state",
      name: "review",
      stateType: "TERMINAL",
      workTypeName: "",
    });
    expect(
      screen.getByText("Choose a work type before adding a work state."),
    ).toBeTruthy();
  });

  it("edits workstation worker assignment and prompt body", () => {
    const onChange = vi.fn();
    const onClose = vi.fn();
    renderDialog({
      draft: {
        behavior: "STANDARD",
        body: "",
        kind: "workstation",
        name: "review",
        workerName: "",
      },
      errors: {
        behavior: "Poller workstations must use a script or hosted worker.",
      },
      onChange,
      onClose,
    });

    fireEvent.change(screen.getByRole("combobox", { name: "Kind" }), {
      target: { value: "POLLER" },
    });
    fireEvent.change(
      screen.getByRole("combobox", { name: "Assigned worker" }),
      {
        target: { value: "writer" },
      },
    );
    const promptBodyInput = getAddDialogPromptBodyInput();
    fireEvent.change(promptBodyInput, {
      target: { value: "Review the draft." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onChange).toHaveBeenNthCalledWith(1, {
      behavior: "POLLER",
      body: "",
      kind: "workstation",
      name: "review",
      workerName: "",
    });
    expect(onChange).toHaveBeenNthCalledWith(2, {
      behavior: "STANDARD",
      body: "",
      kind: "workstation",
      name: "review",
      workerName: "writer",
    });
    expect(onChange).toHaveBeenNthCalledWith(3, {
      behavior: "STANDARD",
      body: "Review the draft.",
      kind: "workstation",
      name: "review",
      workerName: "",
    });
    expect(
      screen.getByText(
        "Poller workstations must use a script or hosted worker.",
      ),
    ).toBeTruthy();
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

function getAddDialogPromptBodyInput() {
  const monacoSurface = document.querySelector(
    '[data-monaco-editor="workstation-prompt"]',
  );
  expect(monacoSurface).toBeTruthy();
  const promptBodyInput = monacoSurface?.querySelector(
    'textarea[aria-label="Prompt body"]',
  );
  expect(promptBodyInput).toBeTruthy();
  return promptBodyInput as HTMLTextAreaElement;
}

function renderDialog({
  draft,
  errors = {},
  isOpen = true,
  onChange = vi.fn(),
  onClose = vi.fn(),
  onSubmit = vi.fn(),
}: {
  draft: FactoryGraphAddEntityDraft | null;
  errors?: Parameters<typeof FactoryGraphEditorAddEntityDialog>[0]["errors"];
  isOpen?: boolean;
  onChange?: Parameters<
    typeof FactoryGraphEditorAddEntityDialog
  >[0]["onChange"];
  onClose?: () => void;
  onSubmit?: () => void;
}) {
  return render(
    <FactoryGraphEditorAddEntityDialog
      currentFactoryDefinition={currentFactoryDefinition}
      draft={draft}
      errors={errors}
      isOpen={isOpen}
      onChange={onChange}
      onClose={onClose}
      onSubmit={onSubmit}
    />,
  );
}
