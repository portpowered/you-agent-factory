import type { CanonicalFactoryDefinition } from "../draft/factory-graph-draft-types";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
} from "../operations/factory-graph-customer-display";

/** Minimal maintainer-shaped factory with runtime-injected system-time routes. */
export const maintainerRuntimeShapedFactory = {
  name: "maintainer-runtime-factory",
  resources: [{ capacity: 10, name: "executor-slot" }],
  workers: [{ name: "processor" }, { name: "workspace-setup" }],
  workTypes: [
    {
      name: "task",
      states: [
        { name: "init", type: "INITIAL" as const },
        { name: "done", type: "TERMINAL" as const },
      ],
    },
    {
      name: SYSTEM_TIME_WORK_TYPE_ID,
      states: [{ name: "pending", type: "PROCESSING" as const }],
    },
  ],
  workstations: [
    {
      inputs: [{ state: "init", workType: "task" }],
      name: "process",
      outputs: [{ state: "done", workType: "task" }],
      resources: [{ capacity: 1, name: "executor-slot" }],
      worker: "processor",
    },
    {
      inputs: [{ state: "init", workType: "task" }],
      name: "setup-workspace",
      outputs: [{ state: "done", workType: "task" }],
      worker: "workspace-setup",
    },
    {
      inputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
      name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
      outputs: [],
      worker: "",
    },
  ],
} satisfies CanonicalFactoryDefinition;
