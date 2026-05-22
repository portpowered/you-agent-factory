import { fireEvent, render, screen } from "@testing-library/react";

import type { CanonicalFactoryDefinition } from "../factory-graph-draft-types";
import { FactoryGraphEditorAddEntityDialog } from "./factory-graph-editor-add-dialog";
import type { FactoryGraphAddEntityDraft } from "../factory-graph-editor-additions";

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

  it("renders worker and work-type specific fields", () => {
    const workerChange = vi.fn();
    const { rerender } = renderDialog({
      draft: { kind: "worker", model: "", name: "writer" },
      errors: { model: "Enter a model identifier for the new worker." },
      onChange: workerChange,
    });

    fireEvent.change(screen.getByRole("textbox", { name: "Model" }), {
      target: { value: "gpt-5.5" },
    });

    expect(workerChange).toHaveBeenCalledWith({
      kind: "worker",
      model: "gpt-5.5",
      name: "writer",
    });
    expect(
      screen.getByText("Enter a model identifier for the new worker."),
    ).toBeTruthy();

    const workTypeChange = vi.fn();
    rerender(
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={currentFactoryDefinition}
        draft={{ initialStateName: "", kind: "work-type", name: "article" }}
        errors={{ initialStateName: "Enter the first work-state identifier." }}
        isOpen={true}
        onChange={workTypeChange}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

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
      errors: { behavior: "Poller workstations must use a script or hosted worker." },
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
    fireEvent.change(screen.getByRole("textbox", { name: "Prompt body" }), {
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
