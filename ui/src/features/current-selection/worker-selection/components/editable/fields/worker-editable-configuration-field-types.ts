import type { EditableWorkerConfigurationState } from "../../../lib/detail-card-types";
import type { getWorkerDetailMessages } from "../../../messages/worker-detail";

export type ReadyWorkerEditableConfigurationState = Extract<
  EditableWorkerConfigurationState,
  { status: "ready" }
>;

export type WorkerEditableConfigurationMessages = ReturnType<
  typeof getWorkerDetailMessages
>;

export type ReadyWorkerEditableValidationErrors =
  ReadyWorkerEditableConfigurationState["validationErrors"];
