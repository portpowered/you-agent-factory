import "../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, render, screen, within } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useEditableWorkTypeConfigurationState } from "../hooks/use-editable-work-type-configuration-state";
import type {
  EditableWorkTypeConfigurationState,
  EditableWorkTypeSaveState,
} from "../lib/detail-card-types";
import { WorkTypeDetailCard } from "./work-type-detail-card";

const CURRENT_SELECTION_FORM_FIELDS_SELECTOR = ".grid.grid-cols-1.gap-3";

import { EditableWorkTypeConfigurationHeaderActions } from "./work-type-save-controls";

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
        modelProvider: "CODEX",
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

function buildWorkTypeHeaderActions({
  canDiscard = false,
  canSave,
  onDiscard = vi.fn(),
  onSave = vi.fn(),
  saveState = { status: "idle" },
}: {
  canDiscard?: boolean;
  canSave: boolean;
  onDiscard?: () => void;
  onSave?: () => void;
  saveState?: EditableWorkTypeSaveState;
}) {
  return (
    <EditableWorkTypeConfigurationHeaderActions
      canDiscard={canDiscard}
      canSave={canSave}
      onDiscard={onDiscard}
      onSave={onSave}
      saveState={saveState}
    />
  );
}

function workTypeDetailHeaderActionSection() {
  const card = screen.getByRole("article", { name: "Current selection" });
  const undoButton = within(card).getByRole("button", {
    name: "Undo selection",
  });
  const actionSection = undoButton.closest(
    "[data-action-row-section='actions']",
  );
  if (!actionSection) {
    throw new Error("expected header action section");
  }

  return actionSection as HTMLElement;
}

function editableWorkTypeConfigurationForm() {
  const panel = screen.getByRole("article", { name: "Current selection" });
  const nameInput = within(panel).getByLabelText("Work type");
  const form = nameInput.closest("form");
  if (!form) {
    throw new Error("expected editable work type configuration form");
  }

  return form;
}

function expectPrimaryWorkTypeTitle(workTypeName: string) {
  const panel = screen.getByRole("article", { name: "Current selection" });
  const title = within(panel).getByText(workTypeName);

  expect(title.classList.contains("type-display-large")).toBe(true);
}

function WorkTypeDetailCardHarness({
  onSelectWorkStateGraphNode,
  workTypeName,
}: {
  onSelectWorkStateGraphNode?: (graphNodeId: string) => void;
  workTypeName: string;
}) {
  const editableDefinition = useCurrentFactoryDocument().data;
  const editableConfigurationState = useEditableWorkTypeConfigurationState(
    { kind: "work-type", workTypeName },
    workTypeName,
    undefined,
    editableDefinition,
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

    expectPrimaryWorkTypeTitle("story");
    expect(
      screen
        .getByRole("button", {
          name: "Collapse work type configuration editor",
        })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    expect(
      screen.getByText(
        "Loading the current factory definition for this work type.",
      ),
    ).toBeTruthy();
  });

  it("renders an error alert when the factory document fails to load", () => {
    render(
      <WorkTypeDetailCard
        editableConfigurationState={{
          errorMessage: "factory offline",
          status: "error",
        }}
        workTypeName="story"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Work type definition unavailable.",
    );
    expect(screen.getByText(/factory offline/)).toBeTruthy();
  });

  it("renders empty guidance when the selected work type is missing", () => {
    render(
      <WorkTypeDetailCard
        editableConfigurationState={{ status: "empty" }}
        workTypeName="missing"
      />,
    );

    expect(
      screen.getByText(
        "This running factory definition does not include the selected work type.",
      ),
    ).toBeTruthy();
  });

  it("renders editable name and handling behavior fields with read-only state rows when ready", async () => {
    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    expectPrimaryWorkTypeTitle("story");
    const nameInput = await within(panel).findByLabelText("Work type");

    expect(nameInput.getAttribute("value")).toBe("story");
    expect(
      within(panel).getByRole("checkbox", { name: "Mark as default work type" })
        .checked,
    ).toBe(false);
    expect(
      within(panel).getByText(
        "Default work type supplies prompt text for simplified factory runs.",
      ),
    ).toBeTruthy();
    expect(within(panel).getByRole("heading", { name: "States" })).toBeTruthy();
    expect(within(panel).getByText("queued")).toBeTruthy();
    expect(within(panel).getByText("Initial")).toBeTruthy();
    expect(within(panel).getByText("done")).toBeTruthy();
    expect(within(panel).getByText("Completed")).toBeTruthy();
  });

  it("surfaces name validation errors with aria-invalid and role alert", async () => {
    render(<WorkTypeDetailCardHarness workTypeName="story" />);

    const panel = screen.getByRole("article", { name: "Current selection" });
    const nameInput = await within(panel).findByLabelText("Work type");

    fireEvent.change(nameInput, { target: { value: "   " } });

    expect(nameInput.getAttribute("aria-invalid")).toBe("true");
    expect(
      within(panel).getByText(
        "Enter a work type name before saving this work type.",
      ),
    ).toBeTruthy();
  });

  it("navigates to the matching work-state graph node when a state row is clicked", async () => {
    const onSelectWorkStateGraphNode = vi.fn();

    render(
      <WorkTypeDetailCardHarness
        onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
        workTypeName="story"
      />,
    );

    fireEvent.click(
      await screen.findByRole("button", {
        name: "Select queued state on factory graph",
      }),
    );

    expect(onSelectWorkStateGraphNode).toHaveBeenCalledWith(
      "work-state:story:queued",
    );
  });

  it("stacks configuration fields vertically and renders header save and discard only when dirty", () => {
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
    const onSave = vi.fn();
    const onDiscard = vi.fn();

    const { container } = render(
      <WorkTypeDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkTypeHeaderActions({
          canDiscard: true,
          canSave: true,
          onDiscard,
          onSave,
        })}
        workTypeName="story"
      />,
    );

    const fieldGroup = container.querySelector(
      CURRENT_SELECTION_FORM_FIELDS_SELECTOR,
    );
    expect(fieldGroup).not.toBeNull();
    expect(fieldGroup?.className).not.toMatch(/sm:grid-cols-\d/);
    expect(fieldGroup?.className).not.toMatch(/md:grid-cols-\d/);
    expect(fieldGroup?.className).not.toMatch(/xl:grid-cols-\d/);

    const headerActions = workTypeDetailHeaderActionSection();
    const saveButtons = within(headerActions).getAllByRole("button", {
      name: "Save changes",
    });
    const discardButtons = within(headerActions).getAllByRole("button", {
      name: "Discard local changes",
    });
    expect(saveButtons).toHaveLength(1);
    expect(discardButtons).toHaveLength(1);

    fireEvent.click(saveButtons[0]);
    expect(onSave).toHaveBeenCalledTimes(1);

    fireEvent.click(discardButtons[0]);
    expect(onDiscard).toHaveBeenCalledTimes(1);

    expect(
      within(editableWorkTypeConfigurationForm()).queryByRole("button", {
        name: "Save changes",
      }),
    ).toBeNull();
    expect(
      within(editableWorkTypeConfigurationForm()).queryByRole("button", {
        name: "Discard local changes",
      }),
    ).toBeNull();
  });

  it("omits global unsaved helper paragraphs for dirty ready-state work type drafts", () => {
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

    render(
      <WorkTypeDetailCard
        editableConfigurationState={editableConfigurationState}
        headerAction={buildWorkTypeHeaderActions({
          canDiscard: true,
          canSave: true,
        })}
        workTypeName="story"
      />,
    );

    expect(
      screen.queryByText("You have unsaved changes for this work type."),
    ).toBeNull();
    expect(
      screen.queryByText(
        "Changes stay local to this edit session until you save the running factory.",
      ),
    ).toBeNull();
    expect(
      screen.getAllByRole("button", { name: "Save changes" }).length,
    ).toBeGreaterThan(0);
  });
});
