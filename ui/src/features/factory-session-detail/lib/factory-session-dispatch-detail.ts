import type {
  FactoryDispatch,
  FactorySessionDispatchDetailRef,
} from "../../../api/factory-sessions/dispatch-detail";

export interface FactorySessionDispatchArtifactLink {
  href: string;
  id: string;
}

export interface FactorySessionDispatchFailureDetailModel {
  errorClass?: string;
  message?: string;
  reason?: string;
}

export interface FactorySessionDispatchJavaScriptModel {
  executionMode?: string;
  taskKind: string;
  taskLabel?: string;
}

export interface FactorySessionDispatchPetriModel {
  transitionId: string;
  workerType?: string;
  workstationName?: string;
}

export interface FactorySessionDispatchDrilldownModel {
  artifactLinks: FactorySessionDispatchArtifactLink[];
  attempt?: number;
  dispatchID: string;
  dispatchKind: string;
  failureDetail?: FactorySessionDispatchFailureDetailModel;
  label?: string;
  model?: string;
  orchestratorKind: string;
  phase?: string;
  promptDigest?: string;
  provider?: string;
  providerSessionRefs: NonNullable<FactoryDispatch["providerSessionRefs"]>;
  relatedWorkIDs: string[];
  runnerID?: string;
  schemaDigest?: string;
  sessionID: string;
  status: string;
  statusHistory: string[];
  usage?: FactoryDispatch["usage"];
  warnings: NonNullable<FactoryDispatch["warnings"]>;
  javascript?: FactorySessionDispatchJavaScriptModel;
  petri?: FactorySessionDispatchPetriModel;
}

export function normalizeFactorySessionDispatchDetail(
  dispatch: FactoryDispatch,
): FactorySessionDispatchDrilldownModel {
  return {
    artifactLinks: (dispatch.artifactIds ?? []).map((artifactID) => ({
      href: buildFactorySessionArtifactHref({
        artifactID,
        session_id: dispatch.sessionId,
      }),
      id: artifactID,
    })),
    attempt: dispatch.attempt,
    dispatchID: dispatch.id,
    dispatchKind: dispatch.dispatchKind,
    failureDetail: dispatch.failureDetail
      ? {
          errorClass: normalizeOptionalText(dispatch.failureDetail.errorClass),
          message: normalizeOptionalText(dispatch.failureDetail.message),
          reason: normalizeOptionalText(dispatch.failureDetail.reason),
        }
      : undefined,
    javascript: dispatch.javascript
      ? {
          executionMode: getJavaScriptExecutionMode(dispatch),
          taskKind: dispatch.javascript.taskKind,
          taskLabel: normalizeOptionalText(dispatch.javascript.taskLabel),
        }
      : undefined,
    label: normalizeOptionalText(dispatch.label),
    model: normalizeOptionalText(dispatch.model),
    orchestratorKind: dispatch.orchestratorKind,
    petri: dispatch.petri
      ? {
          transitionId: dispatch.petri.transitionId,
          workerType: normalizeOptionalText(dispatch.petri.workerType),
          workstationName: normalizeOptionalText(
            dispatch.petri.workstationName,
          ),
        }
      : undefined,
    phase: normalizeOptionalText(dispatch.phase),
    promptDigest: normalizeOptionalText(dispatch.promptDigest),
    provider: normalizeOptionalText(dispatch.provider),
    providerSessionRefs: dispatch.providerSessionRefs ?? [],
    relatedWorkIDs: dispatch.relatedWorkIds ?? [],
    runnerID: normalizeOptionalText(dispatch.runnerId),
    schemaDigest: normalizeOptionalText(dispatch.schemaDigest),
    sessionID: dispatch.sessionId,
    status: dispatch.status,
    statusHistory: getStatusHistory(dispatch),
    usage: dispatch.usage,
    warnings: dispatch.warnings ?? [],
  };
}

function buildFactorySessionArtifactHref({
  artifactID,
  session_id,
}: Pick<FactorySessionDispatchDetailRef, "session_id"> & {
  artifactID: string;
}): string {
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `/factory-sessions/${encodeURIComponent(session_id)}/artifacts/${encodeURIComponent(artifactID)}`;
}

function normalizeOptionalText(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed && trimmed.length > 0 ? trimmed : undefined;
}

function getJavaScriptExecutionMode(
  dispatch: FactoryDispatch,
): string | undefined {
  const javascript = readRecord(readRecord(dispatch).javascript);
  return typeof javascript.executionMode === "string"
    ? normalizeOptionalText(javascript.executionMode)
    : undefined;
}

function getStatusHistory(dispatch: FactoryDispatch): string[] {
  const statusTransitions = readRecord(dispatch).statusTransitions;
  if (!Array.isArray(statusTransitions)) {
    return [];
  }

  return statusTransitions.flatMap((status) => {
    const normalizedStatus =
      typeof status === "string" ? normalizeOptionalText(status) : undefined;
    return normalizedStatus ? [normalizedStatus] : [];
  });
}

function readRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : {};
}
