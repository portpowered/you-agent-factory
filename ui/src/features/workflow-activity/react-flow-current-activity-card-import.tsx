import type { NamedFactoryAPIError } from "../../api/named-factory";
import {
  DashboardButton,
  DashboardMessagePanel,
  DashboardMutationDialog,
} from "../../components/dashboard";
import type {
  FactoryImportActivationState,
  FactoryImportPreviewState,
  FactoryPngDropState,
  ReadFactoryImportPngError,
} from "../import";

const GRAPH_DROP_HINT = "Drop a Port OS factory PNG onto this graph to start import.";
const GRAPH_IMPORT_ERROR_TITLE = "Factory import failed";
const GRAPH_IMPORT_LOADING_TITLE = "Validating factory PNG";
const GRAPH_IMPORT_PREVIEW_TITLE = "Review factory import";

type ReadyFactoryImportPreviewState = Extract<FactoryImportPreviewState, { status: "ready" }>;

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

function factoryImportActivationErrorCopy(error: NamedFactoryAPIError): string {
  switch (error.code) {
    case "FACTORY_ALREADY_EXISTS":
      return "A factory with this name already exists. Rename or remove the existing factory before importing this PNG.";
    case "FACTORY_NOT_IDLE":
      return "The current factory runtime is still active. Wait until it becomes idle before switching factories.";
    case "INVALID_FACTORY":
      return "The dropped factory payload was rejected by the activation API.";
    case "INVALID_FACTORY_NAME":
      return "The embedded factory name is not valid for activation.";
    case "NETWORK_ERROR":
      return "The dashboard could not reach the activation API. Try again once the connection is available.";
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
        <DashboardButton onClick={onDismiss} tone="secondary" type="button">
          Dismiss
        </DashboardButton>
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

function FactoryImportActivationErrorPanel({ error }: { error: NamedFactoryAPIError }) {
  return (
    <DashboardMessagePanel ariaLive="assertive" role="alert" title="Activation failed" tone="error">
      <p className="m-0">{factoryImportActivationErrorCopy(error)}</p>
    </DashboardMessagePanel>
  );
}

interface FactoryImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  onCancel: () => void;
  onConfirm: () => void;
  previewState: ReadyFactoryImportPreviewState;
}

export function FactoryImportPreviewDialog({
  activationState,
  onCancel,
  onConfirm,
  previewState,
}: FactoryImportPreviewDialogProps) {
  const isSubmitting = activationState.status === "submitting";

  return (
    <DashboardMutationDialog
      closeDisabled={isSubmitting}
      closeLabel="Close import preview"
      description={(
        <>
          Review the dropped factory before activation. Confirming this import in the next
          step will switch the current factory to{" "}
          <span className="font-semibold text-af-ink">{previewState.value.factory.name}</span>.
        </>
      )}
      footer={(
        <>
          <DashboardButton
            disabled={isSubmitting}
            onClick={onCancel}
            tone="secondary"
            type="button"
          >
            Cancel import
          </DashboardButton>
          <DashboardButton
            busy={isSubmitting}
            disabled={isSubmitting}
            onClick={onConfirm}
            type="button"
          >
            {isSubmitting ? "Activating factory..." : "Activate factory"}
          </DashboardButton>
        </>
      )}
      media={(
        <div className="overflow-hidden rounded-[1.25rem] border border-af-overlay/10 bg-af-overlay/4 p-3">
          <img
            alt={`${previewState.value.factory.name} preview`}
            className="block h-full max-h-[24rem] w-full rounded-[1rem] object-contain"
            src={previewState.value.previewImageSrc}
          />
        </div>
      )}
      onClose={onCancel}
      overlayClassName="absolute inset-0 z-20 bg-af-ink/16 backdrop-blur-[6px]"
      title={GRAPH_IMPORT_PREVIEW_TITLE}
    >
      <p className="m-0 text-base font-semibold text-af-ink">{previewState.value.factory.name}</p>

      <dl className="grid gap-3 rounded-[1.1rem] border border-af-overlay/10 bg-af-overlay/4 p-4 text-sm text-af-ink/80">
        <div className="grid gap-1">
          <dt className="text-[0.7rem] font-bold uppercase tracking-[0.14em] text-af-accent">
            Dropped file
          </dt>
          <dd className="m-0 font-semibold text-af-ink">{previewState.file.name}</dd>
        </div>
        <div className="grid gap-1">
          <dt className="text-[0.7rem] font-bold uppercase tracking-[0.14em] text-af-accent">
            Embedded factory
          </dt>
          <dd className="m-0 font-semibold text-af-ink">{previewState.value.factory.name}</dd>
        </div>
      </dl>

      {activationState.status === "error" ? (
        <FactoryImportActivationErrorPanel error={activationState.error} />
      ) : null}
    </DashboardMutationDialog>
  );
}
