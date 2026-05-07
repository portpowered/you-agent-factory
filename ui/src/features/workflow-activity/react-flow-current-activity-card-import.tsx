import {
  Button,
} from "../../components/ui";
import {
  DashboardMessagePanel,
} from "./mutation-dialog";
import { getWorkflowActivityGraphImportMessages } from "./messages/graph-import";
import type {
  FactoryPngDropState,
  ReadFactoryImportPngError,
} from "../import";

const defaultMessages = getWorkflowActivityGraphImportMessages(undefined);

export function graphDropStateAttribute(dropState: FactoryPngDropState): string {
  return dropState.status;
}

function graphDropOverlayCopy(dropState: FactoryPngDropState): { message: string; title: string } | null {
  switch (dropState.status) {
    case "drag-active":
      return {
        message: defaultMessages.graphDropHint,
        title: defaultMessages.graphDropTitle,
      };
    case "reading":
      return {
        message: defaultMessages.graphDropReadingMessage(dropState.fileName),
        title: defaultMessages.graphImportLoadingTitle,
      };
    default:
      return null;
  }
}

function graphImportErrorCopy(error: ReadFactoryImportPngError): string {
  switch (error.code) {
    case "NOT_PNG_FILE":
      return defaultMessages.importErrorNotPngFile;
    case "PNG_METADATA_MISSING":
      return defaultMessages.importErrorMetadataMissing;
    case "UNSUPPORTED_SCHEMA_VERSION":
      return defaultMessages.importErrorUnsupportedSchemaVersion(
        error.details?.schemaVersion,
      );
    case "PNG_METADATA_INVALID":
    case "FACTORY_PAYLOAD_INVALID":
      return defaultMessages.importErrorEmbeddedMetadataInvalid;
    case "IMAGE_DECODE_FAILED":
    case "PREVIEW_UNAVAILABLE":
      return defaultMessages.importErrorPreviewUnavailable;
    case "FILE_READ_FAILED":
      return defaultMessages.importErrorFileReadFailed;
    case "PNG_INVALID":
      return defaultMessages.importErrorPngInvalid;
    default:
      return error.message;
  }
}

interface GraphDropOverlayProps {
  dropState: FactoryPngDropState;
}

export function GraphDropOverlay({ dropState }: GraphDropOverlayProps) {
  const copy = graphDropOverlayCopy(dropState);
  if (!copy) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-4 z-10 grid place-items-center rounded-2xl border border-dashed border-af-accent/45 bg-af-surface/92 p-5 text-center shadow-af-panel backdrop-blur-[18px]"
      data-current-activity-drop-overlay={dropState.status}
    >
      <div className="grid max-w-sm gap-2">
        <p className="mb-0 text-xs font-bold uppercase tracking-[0.16em] text-af-accent">
          {copy.title}
        </p>
        <p className="m-0 text-sm text-af-ink/84">{copy.message}</p>
      </div>
    </div>
  );
}

interface GraphImportErrorPanelProps {
  error: ReadFactoryImportPngError;
  fileName: string;
  onDismiss: () => void;
}

export function GraphImportErrorPanel({
  error,
  fileName,
  onDismiss,
}: GraphImportErrorPanelProps) {
  return (
    <DashboardMessagePanel
      action={(
        <Button onClick={onDismiss} tone="outline" type="button">
          Dismiss
        </Button>
      )}
      ariaLive="assertive"
      className="mt-4 min-h-0 px-5 py-4"
      compact={true}
      role="alert"
      title={defaultMessages.graphImportErrorTitle}
      tone="error"
    >
      <p className="m-0">
        <span className="font-semibold">{fileName}</span>
        {" "}
        {graphImportErrorCopy(error)}
      </p>
    </DashboardMessagePanel>
  );
}
