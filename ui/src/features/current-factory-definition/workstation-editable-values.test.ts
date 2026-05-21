import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../api/dashboard/types";
import {
  applyEditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "./workstation-editable-values";

const selectedNode: DashboardWorkstationNode = {
  model: "gpt-5.4",
  node_id: "review",
  transition_id: "review",
  workstation_kind: "MODEL_WORKSTATION",
  workstation_name: "Review",
};

describe("resolveEditableWorkstationValues", () => {
  it("joins the selected workstation with the canonical worker options", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/review.md",
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review the latest story changes before approval.",
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workstationName: "Review",
    });
  });

  it("falls back from transition id lookup to workstation name lookup", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode)?.workstationName,
    ).toBe("Review");
  });

  it("keeps the selected workstation visible when its current worker is missing from the canonical worker list", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "missing-worker",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review the latest story changes before approval.",
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
      workerName: "missing-worker",
      workerOptions: [],
      workstationName: "Review",
    });
  });

  it("applies editable draft changes without rewriting unsupported workstation or worker fields", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          body: "existing worker body",
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          guards: [{ maxVisits: 1, type: "VISIT_COUNT" }],
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          limits: { maxRetries: 3 },
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/review.md",
          stopWords: ["STOP"],
          worker: "reviewer",
          workingDirectory: "/repo/review",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        prompt: "Review the updated prompt before approval.",
        runnerName: null,
        workerName: "reviewer",
      },
    );

    expect(updatedFactory).toMatchObject({
      workers: [
        {
          body: "existing worker body",
          model: "gpt-5.4",
          name: "reviewer",
        },
      ],
      workstations: [
        {
          body: "Review the updated prompt before approval.",
          guards: [{ maxVisits: 1, type: "VISIT_COUNT" }],
          limits: { maxRetries: 3 },
          promptFile: "prompts/review.md",
          stopWords: ["STOP"],
          workingDirectory: "/repo/review",
        },
      ],
    });
  });

  it("returns every current worker as a worker option", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Code work",
          id: "code",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Code",
          outputs: [{ state: "implemented", workType: "story" }],
          worker: "coder",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review work",
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
      workerName: "processor",
      workerOptions: ["processor"],
      workstationName: "Review",
    });
  });

  it("applies worker switches through the workstation field without rewriting worker models", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Code work",
          id: "code",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Code",
          outputs: [{ state: "implemented", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    expect(
      applyEditableWorkstationDraft(factory, selectedNode, {
        prompt: "Updated review work",
        runnerName: "gemini",
        workerName: "reviewer",
      }),
    ).toMatchObject({
      workers: [
        { model: "gpt-5.4", name: "processor" },
        { model: "gpt-5.5", name: "reviewer" },
      ],
      workstations: [
        {
          body: "Updated review work",
          name: "Review",
          runner: "gemini",
          worker: "reviewer",
        },
        { body: "Plan work", name: "Plan" },
        { body: "Code work", name: "Code" },
      ],
    });
  });

  it("rejects worker switches when the selected worker is no longer available", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    expect(
      applyEditableWorkstationDraft(factory, selectedNode, {
        prompt: "Updated review work",
        runnerName: null,
        workerName: "missing-worker",
      }),
    ).toBeNull();
  });
});
