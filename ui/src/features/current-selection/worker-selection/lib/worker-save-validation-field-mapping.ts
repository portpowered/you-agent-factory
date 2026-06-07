import {
  type FactorySaveValidationErrorLike,
  type FactoryValidationTargetLike,
  mapFactoryValidationTargetsToFieldErrors,
} from "../../base/lib/map-factory-validation-target-field-errors";
import type { EditableWorkerSaveValidationErrors } from "./detail-card-types";

const FACTORY_WORKER_RUNTIME_FIELD_PATH =
  /factory\.workers\[\d+\]\.(timeout|stopToken|skipPermissions)/;
const FACTORY_WORKER_HOSTED_LINEAR_FIELD_PATH =
  /factory\.workers\[\d+\]\.((?:auth|linear)(?:\.[a-zA-Z]+)*)/;

const HOSTED_LINEAR_WORKER_TARGET_FIELD_BY_CODE: Record<
  string,
  keyof EditableWorkerSaveValidationErrors
> = {
  "hosted-worker-auth-secret-ref": "authSecretRef",
  "hosted-worker-linear-claim-assignee-field": "linearClaimAssigneeField",
  "hosted-worker-linear-mapping-state": "linearMappingState",
  "hosted-worker-linear-mapping-work-type": "linearMappingWorkType",
};

const HOSTED_LINEAR_WORKER_TARGET_FIELD_BY_SUBJECT_ID: Record<
  string,
  keyof EditableWorkerSaveValidationErrors
> = {
  "auth.secretref": "authSecretRef",
  secretref: "authSecretRef",
  "linear.claim.assigneefield": "linearClaimAssigneeField",
  assigneefield: "linearClaimAssigneeField",
  "linear.mapping.state": "linearMappingState",
  "mapping.state": "linearMappingState",
  "linear.mapping.worktype": "linearMappingWorkType",
  "mapping.worktype": "linearMappingWorkType",
  "linear.pollinterval": "linearPollInterval",
  pollinterval: "linearPollInterval",
  "linear.stateids": "linearStateIds",
  stateids: "linearStateIds",
  "linear.teamids": "linearTeamIds",
  teamids: "linearTeamIds",
};

export function resolveWorkerSaveValidationFieldName(
  target: FactoryValidationTargetLike,
): keyof EditableWorkerSaveValidationErrors | null {
  if (target.subject.type !== "WORKER") {
    return null;
  }

  const hostedLinearField =
    HOSTED_LINEAR_WORKER_TARGET_FIELD_BY_CODE[target.code] ??
    HOSTED_LINEAR_WORKER_TARGET_FIELD_BY_SUBJECT_ID[
      target.subject.id.trim().toLowerCase()
    ];
  if (hostedLinearField) {
    return hostedLinearField;
  }

  const subjectID = target.subject.id.trim().toLowerCase();

  if (subjectID === "type") {
    return "type";
  }
  if (subjectID === "modelprovider") {
    return "modelProvider";
  }
  if (subjectID === "model") {
    return "model";
  }
  if (subjectID === "modellocality") {
    return "modelLocality";
  }
  if (subjectID === "executorprovider") {
    return "executorProvider";
  }
  if (subjectID === "command") {
    return "command";
  }
  if (subjectID === "args") {
    return "args";
  }
  if (subjectID === "body") {
    return "body";
  }
  if (subjectID === "provider") {
    return "provider";
  }
  if (subjectID === "timeout") {
    return "timeout";
  }
  if (subjectID === "stoptoken") {
    return "stopToken";
  }
  if (subjectID === "skippermissions") {
    return "skipPermissions";
  }

  return null;
}

export function mapWorkerSaveErrorToFieldErrors(
  error: FactorySaveValidationErrorLike,
): EditableWorkerSaveValidationErrors | undefined {
  const targetFieldErrors =
    mapFactoryValidationTargetsToFieldErrors(
      error,
      resolveWorkerSaveValidationFieldName,
    ) ?? {};
  const messageFieldErrors = mapWorkerSaveErrorMessageToFieldErrors(
    error.message,
  );

  const fieldErrors = {
    ...targetFieldErrors,
    ...messageFieldErrors,
  };

  return Object.keys(fieldErrors).length > 0 ? fieldErrors : undefined;
}

function mapWorkerSaveErrorMessageToFieldErrors(
  message: string,
): EditableWorkerSaveValidationErrors {
  const hostedLinearFieldMatch = message.match(
    FACTORY_WORKER_HOSTED_LINEAR_FIELD_PATH,
  );
  if (hostedLinearFieldMatch) {
    const [, fieldPath] = hostedLinearFieldMatch;
    const fieldName = resolveHostedLinearWorkerFieldFromPath(fieldPath);
    if (fieldName) {
      return { [fieldName]: message };
    }
  }

  const runtimeFieldMatch = message.match(FACTORY_WORKER_RUNTIME_FIELD_PATH);
  if (!runtimeFieldMatch) {
    return {};
  }

  const [, fieldName] = runtimeFieldMatch;
  switch (fieldName) {
    case "timeout":
      return { timeout: message };
    case "stopToken":
      return { stopToken: message };
    case "skipPermissions":
      return { skipPermissions: message };
    default:
      return {};
  }
}

function resolveHostedLinearWorkerFieldFromPath(
  fieldPath: string,
): keyof EditableWorkerSaveValidationErrors | null {
  switch (fieldPath) {
    case "auth.secretRef":
      return "authSecretRef";
    case "linear.claim.assigneeField":
      return "linearClaimAssigneeField";
    case "linear.mapping.state":
      return "linearMappingState";
    case "linear.mapping.workType":
      return "linearMappingWorkType";
    case "linear.pollInterval":
      return "linearPollInterval";
    case "linear.stateIds":
      return "linearStateIds";
    case "linear.teamIds":
      return "linearTeamIds";
    default:
      return null;
  }
}
