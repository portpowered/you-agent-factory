import type {
  FactorySessionArtifactDetail,
  FactorySessionsAPIError,
} from "../../../api/factory-sessions";
import type { WorkContent } from "../../work-content/public";

export interface FactorySessionArtifactDrilldown {
  artifactId: string;
  auditMode?: FactorySessionArtifactDetail["auditMode"];
  contentHash?: string;
  createdAt?: string;
  dispatchId?: string;
  kind: FactorySessionArtifactDetail["kind"];
  label?: string;
  preview: FactorySessionArtifactDrilldownPreview;
  redactionCounts?: FactorySessionArtifactDetail["redactionCounts"];
  sessionId: string;
  sizeBytes?: number;
  summary?: string;
  visibility: FactorySessionArtifactDetail["visibility"];
  capture?: FactorySessionArtifactDrilldownCapture;
}

export interface FactorySessionArtifactDrilldownCapture {
  capturedAt?: string;
  mimeType?: string;
  sourceDispatchId?: string;
}

export type FactorySessionArtifactDrilldownPreview =
  | {
      content: WorkContent;
      kind: "inline";
    }
  | {
      contentRef: NonNullable<FactorySessionArtifactDetail["contentRef"]>;
      kind: "download";
    }
  | {
      kind: "unavailable";
    };

export type FactorySessionArtifactDrilldownLoadFailure =
  | {
      kind: "invalid-response";
      message?: string;
    }
  | {
      kind: "network";
      message?: string;
    }
  | {
      kind: "not-found";
      message?: string;
    }
  | {
      kind: "unknown";
      message?: string;
    };

export function normalizeFactorySessionArtifactDrilldown(
  artifact: FactorySessionArtifactDetail,
): FactorySessionArtifactDrilldown {
  return {
    artifactId: artifact.id,
    auditMode: artifact.auditMode,
    capture: artifact.captureMetadata
      ? {
          capturedAt: artifact.captureMetadata.capturedAt,
          mimeType: artifact.captureMetadata.mimeType,
          sourceDispatchId: artifact.captureMetadata.sourceDispatchId,
        }
      : undefined,
    contentHash: artifact.contentHash,
    createdAt: artifact.createdAt,
    dispatchId: artifact.dispatchId,
    kind: artifact.kind,
    label: artifact.label,
    preview: normalizeArtifactPreview(artifact),
    redactionCounts: artifact.redactionCounts,
    sessionId: artifact.sessionId,
    sizeBytes: artifact.sizeBytes,
    summary: artifact.summary,
    visibility: artifact.visibility,
  };
}

export function normalizeFactorySessionArtifactDrilldownLoadFailure(
  error: unknown,
): FactorySessionArtifactDrilldownLoadFailure {
  if (!isFactorySessionsAPIError(error)) {
    return {
      kind: "unknown",
      message: error instanceof Error ? error.message : undefined,
    };
  }

  if (error.code === "NETWORK_ERROR") {
    return {
      kind: "network",
      message: error.message,
    };
  }

  if (error.status === 404) {
    return {
      kind: "not-found",
      message: error.message,
    };
  }

  if (
    error.message === "The factory sessions API returned an invalid response."
  ) {
    return {
      kind: "invalid-response",
      message: error.message,
    };
  }

  return {
    kind: "unknown",
    message: error.message,
  };
}

function normalizeArtifactPreview(
  artifact: FactorySessionArtifactDetail,
): FactorySessionArtifactDrilldownPreview {
  if (Array.isArray(artifact.content) && artifact.content.length > 0) {
    return {
      content: artifact.content,
      kind: "inline",
    };
  }

  if (artifact.contentRef) {
    return {
      contentRef: artifact.contentRef,
      kind: "download",
    };
  }

  return {
    kind: "unavailable",
  };
}

function isFactorySessionsAPIError(
  error: unknown,
): error is FactorySessionsAPIError {
  return (
    error instanceof Error &&
    error.name === "FactorySessionsAPIError" &&
    "code" in error
  );
}
