import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard";
import {
  EMPTY_STATE_CLASS,
} from "../../components/dashboard/widget-board";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../../components/ui";
import { cx } from "../../lib/cx";
import type { FactoryPngImportValue } from "./factory-png-import";
import type { FactoryImportActivationState } from "./use-factory-import-activation";
import type { FactoryImportPreviewState } from "./use-factory-import-preview";

const GRAPH_IMPORT_PREVIEW_TITLE = "Review factory import";
const IMPORT_DIALOG_CONTENT_CLASS =
  "w-[min(92vw,60rem)] gap-6 p-5 max-[900px]:p-4 min-[901px]:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]";
const IMPORT_DIALOG_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);
const IMPORT_DIALOG_DESCRIPTION_CLASS = cx("m-0", DASHBOARD_BODY_TEXT_CLASS);
const IMPORT_DIALOG_HINT_CLASS = cx("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS);
const IMPORT_DIALOG_LABEL_CLASS = cx(
  "text-[0.7rem] font-bold uppercase tracking-[0.14em] text-af-accent",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const IMPORT_ERROR_PANEL_CLASS =
  "border-af-danger/30 bg-af-danger/8 text-af-danger-ink";

type ReadyFactoryImportPreviewState = Extract<FactoryImportPreviewState, { status: "ready" }>;

export interface FactoryImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  onCancel: () => void;
  onConfirm: () => void;
  previewState: ReadyFactoryImportPreviewState;
}

export interface DashboardImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  importPreviewState: FactoryImportPreviewState;
  onCancel: () => void;
  onConfirm: (value: FactoryPngImportValue) => void;
}

function factoryImportActivationErrorCopy(error: Extract<FactoryImportActivationState, { status: "error" }>["error"]): string {
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

function FactoryImportActivationErrorPanel({
  error,
}: {
  error: Extract<FactoryImportActivationState, { status: "error" }>["error"];
}) {
  return (
    <div
      aria-live="assertive"
      className={cx(EMPTY_STATE_CLASS, IMPORT_ERROR_PANEL_CLASS)}
      role="alert"
    >
      <div className="grid gap-1">
        <h3>Activation failed</h3>
        <p className={cx("m-0 text-sm", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {factoryImportActivationErrorCopy(error)}
        </p>
      </div>
    </div>
  );
}

export function FactoryImportPreviewDialog({
  activationState,
  onCancel,
  onConfirm,
  previewState,
}: FactoryImportPreviewDialogProps) {
  const isSubmitting = activationState.status === "submitting";
  const handleOpenChange = (open: boolean) => {
    if (!open && !isSubmitting) {
      onCancel();
    }
  };

  return (
    <Dialog onOpenChange={handleOpenChange} open={true}>
      <DialogContent
        className={IMPORT_DIALOG_CONTENT_CLASS}
        closeDisabled={isSubmitting}
        closeLabel="Close import preview"
        onEscapeKeyDown={(event) => {
          if (isSubmitting) {
            event.preventDefault();
          }
        }}
        onInteractOutside={(event) => {
          if (isSubmitting) {
            event.preventDefault();
          }
        }}
      >
        <div className="overflow-hidden rounded-[1.25rem] border border-af-overlay/10 bg-af-overlay/4 p-3">
          <img
            alt={`${previewState.value.factory.name} preview`}
            className="block h-full max-h-[24rem] w-full rounded-[1rem] object-contain"
            src={previewState.value.previewImageSrc}
          />
        </div>
        <div className="grid content-start gap-5">
          <DialogHeader className="grid gap-3">
            <p className={IMPORT_DIALOG_LABEL_CLASS}>Mutation flow</p>
            <div className="grid gap-2">
              <DialogTitle className={IMPORT_DIALOG_TITLE_CLASS}>
                {GRAPH_IMPORT_PREVIEW_TITLE}
              </DialogTitle>
              <DialogDescription className={IMPORT_DIALOG_DESCRIPTION_CLASS}>
                Review the dropped factory before activation. Confirming this import in the
                next step will switch the current factory to{" "}
                <span className="font-semibold text-af-ink">
                  {previewState.value.factory.name}
                </span>
                .
              </DialogDescription>
            </div>
          </DialogHeader>

          <p className="m-0 text-base font-semibold text-af-ink">
            {previewState.value.factory.name}
          </p>

          <dl className="grid gap-3 rounded-[1.1rem] border border-af-overlay/10 bg-af-overlay/4 p-4 text-sm text-af-ink/80">
            <div className="grid gap-1">
              <dt className={IMPORT_DIALOG_LABEL_CLASS}>Dropped file</dt>
              <dd className="m-0 font-semibold text-af-ink">{previewState.file.name}</dd>
            </div>
            <div className="grid gap-1">
              <dt className={IMPORT_DIALOG_LABEL_CLASS}>Embedded factory</dt>
              <dd className="m-0 font-semibold text-af-ink">
                {previewState.value.factory.name}
              </dd>
            </div>
          </dl>

          <p className={IMPORT_DIALOG_HINT_CLASS}>
            Activating the import switches the current dashboard factory to the embedded
            authored definition from this PNG.
          </p>

          {activationState.status === "error" ? (
            <FactoryImportActivationErrorPanel error={activationState.error} />
          ) : null}

          <DialogFooter>
            <Button disabled={isSubmitting} onClick={onCancel} tone="outline" type="button">
              Cancel import
            </Button>
            <Button
              aria-busy={isSubmitting ? "true" : undefined}
              disabled={isSubmitting}
              onClick={onConfirm}
              type="button"
            >
              {isSubmitting ? "Activating factory..." : "Activate factory"}
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function DashboardImportPreviewDialog({
  activationState,
  importPreviewState,
  onCancel,
  onConfirm,
}: DashboardImportPreviewDialogProps) {
  const readyImportPreviewState =
    importPreviewState.status === "ready" ? importPreviewState : null;

  if (!readyImportPreviewState) {
    return null;
  }

  return (
    <FactoryImportPreviewDialog
      activationState={activationState}
      onCancel={onCancel}
      onConfirm={() => {
        onConfirm(readyImportPreviewState.value);
      }}
      previewState={readyImportPreviewState}
    />
  );
}
