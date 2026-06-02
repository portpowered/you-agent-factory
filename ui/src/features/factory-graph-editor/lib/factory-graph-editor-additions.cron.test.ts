import { resolveFactoryGraphAddWorkstationBehaviorOptions } from "../../current-factory-definition/lib/workstation-behavior";
import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import {
  applyFactoryGraphAddEntityDraft,
  createFactoryGraphAddEntityDraft,
  editableWorkstationBehaviorOptions,
  resolveFactoryGraphAddWorkstationDraftForBehaviorChange,
  validateFactoryGraphAddEntityDraft,
} from "./factory-graph-editor-additions";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: cron validation and apply scenarios share one fixture factory.
describe("factory graph editor additions cron and runtime type", () => {
  it("exposes CRON in creation-time behavior options", () => {
    expect(resolveFactoryGraphAddWorkstationBehaviorOptions()).toEqual([
      "STANDARD",
      "REPEATER",
      "POLLER",
      "CRON",
    ]);
    expect(editableWorkstationBehaviorOptions()).toContain("CRON");
  });

  it("preserves existing cron draft when behavior stays CRON", () => {
    const workstationDraft = createFactoryGraphAddEntityDraft(
      "workstation",
      baseFactoryDefinition,
    );
    const cronDraft = resolveFactoryGraphAddWorkstationDraftForBehaviorChange(
      workstationDraft,
      "CRON",
    );
    const filledCron = {
      ...cronDraft,
      cron: {
        expiryWindow: "15m",
        jitter: "1s",
        schedule: "0 * * * *",
        triggerAtStart: true,
      },
    };

    expect(
      resolveFactoryGraphAddWorkstationDraftForBehaviorChange(
        filledCron,
        "CRON",
      ).cron,
    ).toEqual(filledCron.cron);
  });

  it("clears cron when switching add-workstation behavior away from CRON", () => {
    const workstationDraft = createFactoryGraphAddEntityDraft(
      "workstation",
      baseFactoryDefinition,
    );

    const cronDraft = resolveFactoryGraphAddWorkstationDraftForBehaviorChange(
      workstationDraft,
      "CRON",
    );
    expect(cronDraft.cron).toEqual({
      expiryWindow: "",
      jitter: "",
      schedule: "",
      triggerAtStart: false,
    });

    const standardDraft =
      resolveFactoryGraphAddWorkstationDraftForBehaviorChange(
        {
          ...cronDraft,
          cron: {
            expiryWindow: "",
            jitter: "",
            schedule: "0 * * * *",
            triggerAtStart: true,
          },
        },
        "STANDARD",
      );
    expect(standardDraft).toMatchObject({
      behavior: "STANDARD",
      cron: null,
    });
  });

  it("validates cron schedules and skips worker assignment for LOGICAL_MOVE", () => {
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          behavior: "CRON",
          body: "",
          cron: {
            expiryWindow: "",
            jitter: "",
            schedule: "0 * * * *",
            triggerAtStart: true,
          },
          kind: "workstation",
          name: "hourly-router",
          workerName: "",
          workstationType: "LOGICAL_MOVE",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({});

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          behavior: "CRON",
          body: "",
          cron: {
            expiryWindow: "",
            jitter: "",
            schedule: "",
            triggerAtStart: false,
          },
          kind: "workstation",
          name: "hourly-router",
          workerName: "",
          workstationType: "LOGICAL_MOVE",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      cronSchedule: "Enter a cron schedule before adding this workstation.",
    });

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          behavior: "CRON",
          body: "",
          cron: {
            expiryWindow: "",
            jitter: "not-a-duration",
            schedule: "0 * * * *",
            triggerAtStart: false,
          },
          kind: "workstation",
          name: "hourly-cron",
          workerName: "writer",
          workstationType: "MODEL_WORKSTATION",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      cronJitter:
        'Cron jitter "not-a-duration" must be a non-negative Go duration.',
    });
  });

  it("emits cron and runtime type fields for add-workstation drafts", () => {
    const cronModelWorkstation = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        behavior: "CRON",
        body: "",
        cron: {
          expiryWindow: "30m",
          jitter: "5s",
          schedule: "0 * * * *",
          triggerAtStart: true,
        },
        kind: "workstation",
        name: "hourly-review",
        workerName: "writer",
        workstationType: "MODEL_WORKSTATION",
      },
    );
    expect(cronModelWorkstation.additions.workstations).toEqual([
      {
        behavior: "CRON",
        cron: {
          expiryWindow: "30m",
          jitter: "5s",
          schedule: "0 * * * *",
          triggerAtStart: true,
        },
        inputs: [],
        name: "hourly-review",
        outputs: [],
        type: "MODEL_WORKSTATION",
        worker: "writer",
      },
    ]);

    const logicalMoveCron = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        behavior: "CRON",
        body: "",
        cron: {
          expiryWindow: "",
          jitter: "",
          schedule: "@hourly",
          triggerAtStart: false,
        },
        kind: "workstation",
        name: "hourly-router",
        workerName: "",
        workstationType: "LOGICAL_MOVE",
      },
    );
    expect(logicalMoveCron.additions.workstations).toEqual([
      {
        behavior: "CRON",
        cron: {
          schedule: "@hourly",
          triggerAtStart: false,
        },
        inputs: [],
        name: "hourly-router",
        outputs: [],
        type: "LOGICAL_MOVE",
        worker: "",
      },
    ]);
  });

  it("omits stale cron when behavior is not CRON", () => {
    const nextDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        behavior: "STANDARD",
        body: "",
        cron: {
          expiryWindow: "",
          jitter: "",
          schedule: "0 * * * *",
          triggerAtStart: false,
        },
        kind: "workstation",
        name: "plain",
        workerName: "writer",
        workstationType: "MODEL_WORKSTATION",
      },
    );

    expect(nextDraft.additions.workstations).toEqual([
      {
        inputs: [],
        name: "plain",
        outputs: [],
        type: "MODEL_WORKSTATION",
        worker: "writer",
      },
    ]);
  });
});
