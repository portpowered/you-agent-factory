import { useId, useMemo, useState } from "react";

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
import { cn } from "../../../lib/cn";
import type { FactoryPngImportValue } from "../lib/factory-png-import";
import { allocateImportCreateFactoryName } from "../lib/allocate-import-create-factory-name";
import type {
  FactoryImportConfirmInput,
  FactoryImportSaveChoice,
} from "../lib/factory-import-save-choice";
import {
  getImportPreviewDialogMessages,
  IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN,
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
const IMPORT_SAVE_CHOICE_OPTION_CLASS =
  "grid cursor-pointer gap-1 rounded-xl border border-transparent p-3 transition-colors has-[:focus-visible]:border-af-accent has-[:checked]:border-af-accent has-[:checked]:bg-af-surface";
const IMPORT_ERROR_PANEL_CLASS =
  "border-af-danger-border bg-af-danger-surface text-af-danger-text";

type ReadyFactoryImportPreviewState = Extract<FactoryImportPreviewState, { status: "ready" }>;

export interface FactoryImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  currentSessionFactoryName: string;
  existingFactoryNames?: readonly string[];
  locale?: string;
  onCancel: () => void;
  onConfirm: (input: FactoryImportConfirmInput) => void;
  previewState: ReadyFactoryImportPreviewState;
}

export interface DashboardImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  currentSessionFactoryName: string;
  existingFactoryNames?: readonly string[];
  importPreviewState: FactoryImportPreviewState;
  locale?: string;
  onCancel: () => void;
  onConfirm: (input: FactoryImportConfirmInput) => void;
}

function renderImportPreviewCurrentFactoryDescription(
  template: string,
  currentFactoryName: string,
) {
  const [beforeFactoryName, afterFactoryName] = template.split(
    IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN,
  );

  if (afterFactoryName === undefined) {
    return template;
  }

  return (
    <>
      {beforeFactoryName}
      <span className="font-semibold text-af-text">{currentFactoryName}</span>
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
    case "STALE_FACTORY_VERSION":
      return messages.errorByCode.STALE_FACTORY_VERSION;
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

function collectExistingFactoryNames(
  currentSessionFactoryName: string,
  embeddedFactoryName: string,
  existingFactoryNames: readonly string[] | undefined,
): string[] {
  const names = new Set<string>();

  for (const candidate of [
    currentSessionFactoryName,
    embeddedFactoryName,
    ...(existingFactoryNames ?? []),
  ]) {
    const normalized = candidate.trim();
    if (normalized.length > 0) {
      names.add(normalized);
    }
  }

  return [...names];
}

function FactoryImportSaveChoiceFieldset({
  choice,
  createFactoryName,
  currentSessionFactoryName,
  isSubmitting,
  locale,
  onChoiceChange,
}: {
  choice: FactoryImportSaveChoice;
  createFactoryName: string;
  currentSessionFactoryName: string;
  isSubmitting: boolean;
  locale?: string;
  onChoiceChange: (choice: FactoryImportSaveChoice) => void;
}) {
  const messages = getImportPreviewDialogMessages(locale);
  const replaceOptionId = useId();
  const createOptionId = useId();

  return (
    <fieldset
      className="grid gap-3 rounded-2xl border border-af-border bg-af-surface-subtle p-4"
      disabled={isSubmitting}
    >
      <legend className={IMPORT_DIALOG_LABEL_CLASS}>{messages.saveChoiceLegend}</legend>
      <div className="grid gap-2" role="radiogroup" aria-label={messages.saveChoiceLegend}>
        <label className={IMPORT_SAVE_CHOICE_OPTION_CLASS} htmlFor={replaceOptionId}>
          <span className="flex items-start gap-3">
            <input
              checked={choice === "replace_current"}
              className="mt-1"
              id={replaceOptionId}
              name="factory-import-save-choice"
              onChange={() => {
                onChoiceChange("replace_current");
              }}
              type="radio"
              value="replace_current"
            />
            <span className="grid gap-1">
              <span className="text-base font-semibold text-af-text">
                {messages.replaceCurrentOption}
              </span>
              <span className={IMPORT_DIALOG_HINT_CLASS}>
                {messages.replaceCurrentOptionDescription}
              </span>
              <span className="text-sm font-semibold text-af-text">
                {currentSessionFactoryName}
              </span>
            </span>
          </span>
        </label>
        <label className={IMPORT_SAVE_CHOICE_OPTION_CLASS} htmlFor={createOptionId}>
          <span className="flex items-start gap-3">
            <input
              checked={choice === "create_new_named"}
              className="mt-1"
              id={createOptionId}
              name="factory-import-save-choice"
              onChange={() => {
                onChoiceChange("create_new_named");
              }}
              type="radio"
              value="create_new_named"
            />
            <span className="grid gap-1">
              <span className="text-base font-semibold text-af-text">
                {messages.createNewNamedOption}
              </span>
              <span className={IMPORT_DIALOG_HINT_CLASS}>
                {messages.createNewNamedOptionDescription}
              </span>
              <span className="grid gap-1">
                <span className={IMPORT_DIALOG_LABEL_CLASS}>
                  {messages.createResolvedNameLabel}
                </span>
                <span className="text-sm font-semibold text-af-text">{createFactoryName}</span>
              </span>
            </span>
          </span>
        </label>
      </div>
    </fieldset>
  );
}

export function FactoryImportPreviewDialog({
  activationState,
  currentSessionFactoryName,
  existingFactoryNames,
  locale,
  onCancel,
  onConfirm,
  previewState,
}: FactoryImportPreviewDialogProps) {
  const isSubmitting = activationState.status === "submitting";
  const messages = getImportPreviewDialogMessages(locale);
  const [choice, setChoice] = useState<FactoryImportSaveChoice>("replace_current");
  const resolvedExistingFactoryNames = useMemo(
    () =>
      collectExistingFactoryNames(
        currentSessionFactoryName,
        previewState.value.factory.name,
        existingFactoryNames,
      ),
    [currentSessionFactoryName, existingFactoryNames, previewState.value.factory.name],
  );
  const createFactoryName = useMemo(
    () =>
      allocateImportCreateFactoryName(
        previewState.value.factory.name,
        resolvedExistingFactoryNames,
      ),
    [previewState.value.factory.name, resolvedExistingFactoryNames],
  );

  const handleOpenChange = (open: boolean) => {
    if (!open && !isSubmitting) {
      onCancel();
    }
  };

  const handleConfirm = () => {
    onConfirm({
      choice,
      createFactoryName,
      existingFactoryNames: resolvedExistingFactoryNames,
      value: previewState.value,
    });
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
                {renderImportPreviewCurrentFactoryDescription(
                  messages.descriptionTemplate,
                  currentSessionFactoryName,
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

          <FactoryImportSaveChoiceFieldset
            choice={choice}
            createFactoryName={createFactoryName}
            currentSessionFactoryName={currentSessionFactoryName}
            isSubmitting={isSubmitting}
            locale={locale}
            onChoiceChange={setChoice}
          />

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
              onClick={handleConfirm}
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
  currentSessionFactoryName,
  existingFactoryNames,
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
      key={readyImportPreviewState.file.name}
      activationState={activationState}
      currentSessionFactoryName={currentSessionFactoryName}
      existingFactoryNames={existingFactoryNames}
      locale={locale}
      onCancel={onCancel}
      onConfirm={onConfirm}
      previewState={readyImportPreviewState}
    />
  );
}
