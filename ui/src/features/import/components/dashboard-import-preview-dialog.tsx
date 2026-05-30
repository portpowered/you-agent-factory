import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  EMPTY_STATE_CLASS,
} from "../../../components/ui/widget-frame";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../../../components/ui";
import { SelectableCardButton } from "../../../components/ui/selectable-card-button";
import type { FactoryImportSaveChoice } from "../../../api/named-factory";
import { cn } from "../../../lib/cn";
import type { FactoryPngImportValue } from "../lib/factory-png-import";
import {
  getImportPreviewDialogMessages,
  IMPORT_PREVIEW_FACTORY_NAME_TOKEN,
} from "../messages/import-preview-dialog";
import type { FactoryImportActivationState } from "../hooks/use-factory-import-activation";
import type { FactoryImportPreviewState } from "../hooks/use-factory-import-preview";

const IMPORT_DIALOG_CONTENT_CLASS =
  "w-full max-w-5xl gap-6 p-4 md:grid-cols-[minmax(0,22rem)_minmax(0,1fr)] md:p-5";
const IMPORT_DIALOG_TITLE_CLASS = cn("m-0", DASHBOARD_SECTION_HEADING_CLASS);
const IMPORT_DIALOG_DESCRIPTION_CLASS = cn("m-0", DASHBOARD_BODY_TEXT_CLASS);
const IMPORT_DIALOG_HINT_CLASS = cn("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS);
const IMPORT_DIALOG_LABEL_CLASS = cn(
  "text-[0.7rem] font-bold uppercase tracking-[0.14em] text-af-accent",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const IMPORT_ERROR_PANEL_CLASS =
  "border-af-danger-border bg-af-danger-surface text-af-danger-text";
const IMPORT_DIALOG_MODE_OPTIONS_CLASS = "grid gap-3";
const IMPORT_DIALOG_MODE_OPTION_CLASS =
  "grid w-full gap-2 rounded-2xl border border-af-border bg-af-surface-subtle p-4 text-left";
const IMPORT_DIALOG_MODE_OPTION_SELECTED_CLASS =
  "border-af-accent bg-af-surface";

type ReadyFactoryImportPreviewState = Extract<FactoryImportPreviewState, { status: "ready" }>;

export interface FactoryImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  createTargetFactoryName?: string | null;
  currentFactoryName?: string | null;
  importSaveChoice: FactoryImportSaveChoice;
  locale?: string;
  onCancel: () => void;
  onConfirm: () => void;
  onImportSaveChoiceChange: (choice: FactoryImportSaveChoice) => void;
  previewState: ReadyFactoryImportPreviewState;
}

export interface DashboardImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  createTargetFactoryName?: string | null;
  currentFactoryName?: string | null;
  importPreviewState: FactoryImportPreviewState;
  importSaveChoice: FactoryImportSaveChoice;
  locale?: string;
  onCancel: () => void;
  onConfirm: (value: FactoryPngImportValue, choice: FactoryImportSaveChoice) => void;
  onImportSaveChoiceChange: (choice: FactoryImportSaveChoice) => void;
  sessionID?: string | null;
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
      <span className="font-semibold text-af-text">{factoryName}</span>
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
      className={cn(EMPTY_STATE_CLASS, IMPORT_ERROR_PANEL_CLASS)}
      role="alert"
    >
      <div className="grid gap-1">
        <h3>{messages.activationErrorTitle}</h3>
        <p className={cn("m-0 text-sm", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {factoryImportActivationErrorCopy(error, locale)}
        </p>
      </div>
    </div>
  );
}

export function FactoryImportPreviewDialog({
  activationState,
  createTargetFactoryName,
  currentFactoryName,
  importSaveChoice,
  locale,
  onCancel,
  onConfirm,
  onImportSaveChoiceChange,
  previewState,
}: FactoryImportPreviewDialogProps) {
  const isSubmitting = activationState.status === "submitting";
  const messages = getImportPreviewDialogMessages(locale);
  const resolvedCurrentFactoryName =
    currentFactoryName?.trim() || previewState.value.factory.name;
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
        <div className="overflow-hidden rounded-3xl border border-af-border bg-af-surface-subtle p-3">
          <img
            alt={messages.previewImageAlt(previewState.value.factory.name)}
            className="block h-full max-h-96 w-full rounded-2xl object-contain"
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

          <p className="m-0 text-base font-semibold text-af-text">
            {previewState.value.factory.name}
          </p>

          <dl className="grid gap-3 rounded-2xl border border-af-border bg-af-surface-subtle p-4 text-sm text-af-text-muted">
            <div className="grid gap-1">
              <dt className={IMPORT_DIALOG_LABEL_CLASS}>{messages.droppedFileLabel}</dt>
              <dd className="m-0 font-semibold text-af-text">{previewState.file.name}</dd>
            </div>
            <div className="grid gap-1">
              <dt className={IMPORT_DIALOG_LABEL_CLASS}>{messages.embeddedFactoryLabel}</dt>
              <dd className="m-0 font-semibold text-af-text">
                {previewState.value.factory.name}
              </dd>
            </div>
          </dl>

          <fieldset className={IMPORT_DIALOG_MODE_OPTIONS_CLASS} disabled={isSubmitting}>
            <legend className={IMPORT_DIALOG_LABEL_CLASS}>{messages.importSaveChoiceLegend}</legend>
            <SelectableCardButton
              className={cn(
                IMPORT_DIALOG_MODE_OPTION_CLASS,
                importSaveChoice === "REPLACE_CURRENT" &&
                  IMPORT_DIALOG_MODE_OPTION_SELECTED_CLASS,
              )}
              disabled={isSubmitting}
              onClick={() => {
                onImportSaveChoiceChange("REPLACE_CURRENT");
              }}
              selected={importSaveChoice === "REPLACE_CURRENT"}
              tone="outline"
              type="button"
            >
              <span className="font-semibold text-af-text">
                {messages.replaceCurrentFactoryLabel}
              </span>
              <span className={IMPORT_DIALOG_HINT_CLASS}>
                {messages.replaceCurrentFactoryDescription(resolvedCurrentFactoryName)}
              </span>
            </SelectableCardButton>
            <SelectableCardButton
              className={cn(
                IMPORT_DIALOG_MODE_OPTION_CLASS,
                importSaveChoice === "CREATE_NEW_NAMED" &&
                  IMPORT_DIALOG_MODE_OPTION_SELECTED_CLASS,
              )}
              disabled={isSubmitting}
              onClick={() => {
                onImportSaveChoiceChange("CREATE_NEW_NAMED");
              }}
              selected={importSaveChoice === "CREATE_NEW_NAMED"}
              tone="outline"
              type="button"
            >
              <span className="font-semibold text-af-text">
                {messages.createNewNamedFactoryLabel}
              </span>
              <span className={IMPORT_DIALOG_HINT_CLASS}>
                {messages.createNewNamedFactoryDescription}
              </span>
              {createTargetFactoryName ? (
                <span className="m-0 text-sm font-semibold text-af-text">
                  {messages.createNewNamedFactoryResolvedNameLabel}: {createTargetFactoryName}
                </span>
              ) : null}
            </SelectableCardButton>
          </fieldset>

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
  createTargetFactoryName,
  currentFactoryName,
  importPreviewState,
  importSaveChoice,
  locale,
  onCancel,
  onConfirm,
  onImportSaveChoiceChange,
}: DashboardImportPreviewDialogProps) {
  const readyImportPreviewState =
    importPreviewState.status === "ready" ? importPreviewState : null;

  if (!readyImportPreviewState) {
    return null;
  }

  return (
    <FactoryImportPreviewDialog
      activationState={activationState}
      createTargetFactoryName={createTargetFactoryName}
      currentFactoryName={currentFactoryName}
      importSaveChoice={importSaveChoice}
      locale={locale}
      onCancel={onCancel}
      onConfirm={() => {
        onConfirm(readyImportPreviewState.value, importSaveChoice);
      }}
      onImportSaveChoiceChange={onImportSaveChoiceChange}
      previewState={readyImportPreviewState}
    />
  );
}
