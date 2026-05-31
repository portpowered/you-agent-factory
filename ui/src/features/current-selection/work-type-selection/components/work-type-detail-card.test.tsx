import { fireEvent, render, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS } from "../../base/components/detail-card-shared";
import { useEditableWorkTypeConfigurationState } from "../hooks/use-editable-work-type-configuration-state";
import type {
  EditableWorkTypeConfigurationState,
  EditableWorkTypeSaveState,
} from "../lib/detail-card-types";
import { WorkTypeDetailCard } from "./work-type-detail-card";
import { EditableWorkTypeSaveHeaderAction } from "./work-type-save-controls";

vi.mock(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

function buildFactoryDocument(
  overrides?: Partial<CurrentFactoryDocument>,
): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    version: {
      logical: "7",
      physical: "2026-05-23T16:22:24Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        modelProvider: "CURSOR",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        id: "review",
        inputs: [],
        name: "Review",
        worker: "reviewer",
      },
    ],
    workTypes: [
      {
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
    ],
    ...overrides,
  };
}

function buildWorkTypeHeaderSaveAction({
  canSave,
  onClick = vi.fn(),
  saveState = { status: "idle" },
}: {
  canSave: boolean;
  onClick?: () => void;
  saveState?: EditableWorkTypeSaveState;
}) {
  return (
    <EditableWorkTypeSaveHeaderAction
      canSave={canSave}
      onClick={onClick}
      saveState={saveState}
    />
  );
}

function WorkTypeDetailCardHarness({
  onSelectWorkStateGraphNode,
  workTypeName,
}: {
  onSelectWorkStateGraphNode?: (graphNodeId: string) => void;
  workTypeName: string;
}) {
  const editableConfigurationState = useEditableWorkTypeConfigurationState(
    { kind: "work-type", workTypeName },
    workTypeName,
  );

  return (
    <WorkTypeDetailCard
      editableConfigurationState={editableConfigurationState}
      onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
      workTypeName={workTypeName}
    />
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: WorkTypeDetailCard coverage keeps loading, ready, and editable state regressions together.
describe("WorkTypeDetailCard", () => {
  beforeEach(() => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: buildFactoryDocument(),
      error: null,
      isError: false,
      isPending: false,
      status: "success",
    } as never);
  });

  it("renders a loading message while the factory document is pending", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      isError: false,
      isPending: true,
      status: "pending",
    } as never);

    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    expect(
      screen.getByText(
        "Loading the current factory definition for this work type.",
      ),
    ).toBeTruthy();
  });

  it("renders an error alert when the factory document fails to load", () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: new Error("factory offline"),
      isError: true,
      isPending: false,
      status: "error",
    } as never);

    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    expect(screen.getByRole("alert").textContent).toContain(
      "Work type definition unavailable.",
    );
    expect(screen.getByText(/factory offline/)).toBeTruthy();
  });

  it("renders empty guidance when the selected work type is missing", () => {
    render(<WorkTypeDetailCardHarness workTypeName="missing" />);

    expect(
      screen.getByText(
        "This running factory definition does not include the selected work type.",
      ),
    ).toBeTruthy();
  });

  it("renders editable name and handling behavior fields with read-only state rows when ready", () => {
    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    const nameInput = within(panel).getByLabelText("Work type");

    expect(nameInput.getAttribute("value")).toBe("story");
    expect(
      within(panel).getByRole("checkbox", { name: "Default CLI handling" })
        .checked,
    ).toBe(false);
    expect(within(panel).getByRole("heading", { name: "States" })).toBeTruthy();
    expect(within(panel).getByText("queued")).toBeTruthy();
    expect(within(panel).getByText("Initial")).toBeTruthy();
    expect(within(panel).getByText("done")).toBeTruthy();
    expect(within(panel).getByText("Completed")).toBeTruthy();
  });

  it("surfaces name validation errors with aria-invalid and role alert", () => {
    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    const nameInput = within(panel).getByLabelText("Work type");

    fireEvent.change(nameInput, { target: { value: "   " } });

    expect(nameInput.getAttribute("aria-invalid")).toBe("true");
    expect(
      within(panel).getByText(
        "Enter a work type name before saving this work type.",
      ),
    ).toBeTruthy();
  });

  it("navigates to the matching work-state graph node when a state row is clicked", () => {
    const onSelectWorkStateGraphNode = vi.fn();

    render(
      <WorkTypeDetailCardHarness
        onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
        workTypeName="story"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: "Select queued state on factory graph",
      }),
    );

    expect(onSelectWorkStateGraphNode).toHaveBeenCalledWith(
      "work-state:story:queued",
    );
  });

  it("stacks configuration fields vertically and renders a labeled footer Save", () => {
    const editableConfigurationState: EditableWorkTypeConfigurationState = {
      baseVersion: buildFactoryDocument().version,
      canSave: true,
      draft: {
        handlingBehavior: null,
        name: "story",
      },
      hasValidationErrors: false,
      initialValues: {
        handlingBehavior: null,
        name: "story",
        states: [
          { name: "queued", type: "INITIAL" },
          { name: "done", type: "TERMINAL" },
        ],
      },
      isDirty: true,
      markChangesSaved: vi.fn(),
      onHandlingBehaviorChange: vi.fn(),
      onNameChange: vi.fn(),
      onResetToLatest: vi.fn(),
      pendingFactoryDefinition: buildFactoryDocument(),
      status: "ready",
      validationErrors: {},
    };
    const onSaveConfiguration = vi.fn();

    const { container } = render(
      <WorkTypeDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkTypeHeaderSaveAction({ canSave: true })}
        onSaveConfiguration={onSaveConfiguration}
        workTypeName="story"
      />,
    );

    const panel = screen.getByRole("article", { name: "Current selection" });
    const fieldGroup = container.querySelector(
      `.${CURRENT_SELECTION_VERTICAL_FORM_FIELDS_CLASS.replaceAll(" ", ".")}`,
    );
    expect(fieldGroup).not.toBeNull();
    expect(fieldGroup?.className).not.toMatch(/sm:grid-cols-\d/);
    expect(fieldGroup?.className).not.toMatch(/md:grid-cols-\d/);
    expect(fieldGroup?.className).not.toMatch(/xl:grid-cols-\d/);

    const saveButtons = within(panel).getAllByRole("button", {
      name: "Save changes",
    });
    expect(saveButtons).toHaveLength(2);

    fireEvent.click(saveButtons[1] ?? saveButtons[0]);
    expect(onSaveConfiguration).toHaveBeenCalledTimes(1);
  });
});
