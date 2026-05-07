import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/ui/dashboard-typography";
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
import {
  getImportPreviewDialogMessages,
  IMPORT_PREVIEW_FACTORY_NAME_TOKEN,
} from "./messages/import-preview-dialog";
import type { FactoryImportActivationState } from "./use-factory-import-activation";
import type { FactoryImportPreviewState } from "./use-factory-import-preview";

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
  locale?: string;
  onCancel: () => void;
  onConfirm: () => void;
  previewState: ReadyFactoryImportPreviewState;
}

export interface DashboardImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  importPreviewState: FactoryImportPreviewState;
  locale?: string;
  onCancel: () => void;
  onConfirm: (value: FactoryPngImportValue) => void;
}

function renderImportPreviewDescription(template: string, factoryName: string) {
  const [beforeFactoryName, afterFactoryName] = template.split(
    IMPORT_PREVIEW_FACTORY_NAME_TOKEN,
  );

  if (afterFactoryName === undefined) {
    return template;
  }

  return (
    <>
      {beforeFactoryName}
      <span className="font-semibold text-af-ink">{factoryName}</span>
      {afterFactoryName}
    </>
  );
}

function factoryImportActivationErrorCopy(
  error: Extract<FactoryImportActivationState, { status: "error" }>["error"],
  locale?: string,
): string {
  const messages = getImportPreviewDialogMessages(locale);

  switch (error.code) {
    case "FACTORY_ALREADY_EXISTS":
      return messages.errorByCode.FACTORY_ALREADY_EXISTS;
    case "FACTORY_NOT_IDLE":
      return messages.errorByCode.FACTORY_NOT_IDLE;
    case "INVALID_FACTORY":
      return messages.errorByCode.INVALID_FACTORY;
    case "INVALID_FACTORY_NAME":
      return messages.errorByCode.INVALID_FACTORY_NAME;
    case "NETWORK_ERROR":
      return messages.errorByCode.NETWORK_ERROR;
    default:
      return error.message;
  }
}

function FactoryImportActivationErrorPanel({
  error,
  locale,
}: {
  error: Extract<FactoryImportActivationState, { status: "error" }>["error"];
  locale?: string;
}) {
  const messages = getImportPreviewDialogMessages(locale);

  return (
    <div
      aria-live="assertive"
      className={cx(EMPTY_STATE_CLASS, IMPORT_ERROR_PANEL_CLASS)}
      role="alert"
    >
      <div className="grid gap-1">
        <h3>{messages.activationErrorTitle}</h3>
        <p className={cx("m-0 text-sm", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {factoryImportActivationErrorCopy(error, locale)}
        </p>
      </div>
    </div>
  );
}

export function FactoryImportPreviewDialog({
  activationState,
  locale,
  onCancel,
  onConfirm,
  previewState,
}: FactoryImportPreviewDialogProps) {
  const isSubmitting = activationState.status === "submitting";
  const messages = getImportPreviewDialogMessages(locale);
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
        closeLabel={messages.closeLabel}
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
            alt={messages.previewImageAlt(previewState.value.factory.name)}
            className="block h-full max-h-[24rem] w-full rounded-[1rem] object-contain"
            src={previewState.value.previewImageSrc}
          />
        </div>
        <div className="grid content-start gap-5">
          <DialogHeader className="grid gap-3">
            <p className={IMPORT_DIALOG_LABEL_CLASS}>{messages.flowLabel}</p>
            <div className="grid gap-2">
              <DialogTitle className={IMPORT_DIALOG_TITLE_CLASS}>
                {messages.title}
              </DialogTitle>
              <DialogDescription className={IMPORT_DIALOG_DESCRIPTION_CLASS}>
                {renderImportPreviewDescription(
                  messages.descriptionTemplate,
                  previewState.value.factory.name,
                )}
              </DialogDescription>
            </div>
          </DialogHeader>

          <p className="m-0 text-base font-semibold text-af-ink">
            {previewState.value.factory.name}
          </p>

          <dl className="grid gap-3 rounded-[1.1rem] border border-af-overlay/10 bg-af-overlay/4 p-4 text-sm text-af-ink/80">
            <div className="grid gap-1">
              <dt className={IMPORT_DIALOG_LABEL_CLASS}>{messages.droppedFileLabel}</dt>
              <dd className="m-0 font-semibold text-af-ink">{previewState.file.name}</dd>
            </div>
            <div className="grid gap-1">
              <dt className={IMPORT_DIALOG_LABEL_CLASS}>{messages.embeddedFactoryLabel}</dt>
              <dd className="m-0 font-semibold text-af-ink">
                {previewState.value.factory.name}
              </dd>
            </div>
          </dl>

          <p className={IMPORT_DIALOG_HINT_CLASS}>{messages.hint}</p>

          {activationState.status === "error" ? (
            <FactoryImportActivationErrorPanel error={activationState.error} locale={locale} />
          ) : null}

          <DialogFooter>
            <Button disabled={isSubmitting} onClick={onCancel} tone="outline" type="button">
              {messages.cancelAction}
            </Button>
            <Button
              aria-busy={isSubmitting ? "true" : undefined}
              disabled={isSubmitting}
              onClick={onConfirm}
              type="button"
            >
              {isSubmitting ? messages.activatingAction : messages.activateAction}
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
  locale,
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
      locale={locale}
      onCancel={onCancel}
      onConfirm={() => {
        onConfirm(readyImportPreviewState.value);
      }}
      previewState={readyImportPreviewState}
    />
  );
}
