import type {
  FactoryDispatch,
  FactorySessionDispatchDetailRef,
} from "../../../api/factory-sessions/dispatch-detail";

export interface FactorySessionDispatchArtifactLink {
  href: string;
  id: string;
}

export interface FactorySessionDispatchFailureDetailModel {
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
          message: normalizeOptionalText(dispatch.failureDetail.message),
          reason: normalizeOptionalText(dispatch.failureDetail.reason),
        }
      : undefined,
    javascript: dispatch.javascript
      ? {
          executionMode: normalizeOptionalText(
            dispatch.javascript.executionMode,
          ),
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
    statusHistory: (dispatch.statusTransitions ?? []).flatMap((status) => {
      const normalizedStatus = normalizeOptionalText(status);
      return normalizedStatus ? [normalizedStatus] : [];
    }),
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
