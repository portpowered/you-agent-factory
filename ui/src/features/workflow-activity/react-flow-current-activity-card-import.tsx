import {
  Button,
} from "../../components/ui";
import {
  DashboardMessagePanel,
} from "./mutation-dialog";
import type {
  FactoryPngDropState,
  ReadFactoryImportPngError,
} from "../import";

const GRAPH_DROP_HINT = "Drop a Port OS factory PNG onto this graph to start import.";
const GRAPH_IMPORT_ERROR_TITLE = "Factory import failed";
const GRAPH_IMPORT_LOADING_TITLE = "Validating factory PNG";

export function graphDropStateAttribute(dropState: FactoryPngDropState): string {
  return dropState.status;
}

function graphDropOverlayCopy(dropState: FactoryPngDropState): { message: string; title: string } | null {
  switch (dropState.status) {
    case "drag-active":
      return {
        message: GRAPH_DROP_HINT,
        title: "Import factory PNG",
      };
    case "reading":
      return {
        message: `${dropState.fileName} is being parsed and validated locally before import continues.`,
        title: GRAPH_IMPORT_LOADING_TITLE,
      };
    default:
      return null;
  }
}

function graphImportErrorCopy(error: ReadFactoryImportPngError): string {
  switch (error.code) {
    case "NOT_PNG_FILE":
      return "Drop a PNG image exported by Port OS Agent Factory.";
    case "PNG_METADATA_MISSING":
      return "This PNG does not include the Port OS factory metadata needed for import.";
    case "UNSUPPORTED_SCHEMA_VERSION":
      return error.details?.schemaVersion
        ? `This PNG uses unsupported Port OS factory metadata version ${error.details.schemaVersion}.`
        : "This PNG uses an unsupported Port OS factory metadata version.";
    case "PNG_METADATA_INVALID":
    case "FACTORY_PAYLOAD_INVALID":
      return "The embedded Port OS factory metadata is invalid, so the current factory was left unchanged.";
    case "IMAGE_DECODE_FAILED":
    case "PREVIEW_UNAVAILABLE":
      return "The browser could not validate this PNG for import preview, so the current factory was left unchanged.";
    case "FILE_READ_FAILED":
      return "The browser could not read the dropped file. Try dropping the PNG again.";
    case "PNG_INVALID":
      return "This PNG appears truncated or malformed, so import stopped before any activation request.";
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
      title={GRAPH_IMPORT_ERROR_TITLE}
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
