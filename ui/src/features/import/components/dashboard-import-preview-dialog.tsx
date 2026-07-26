import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@you-agent-factory/components/overlays";
import {
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
} from "@you-agent-factory/components/recipes";
import { useId, useMemo, useState } from "react";
import { DescriptionList } from "@you-agent-factory/components/data-display";
import { SurfacePanel } from "@you-agent-factory/components/layout";
import { Button, Heading, Label, Text } from "@you-agent-factory/components/primitives";
import { AlertPanel } from "../../../components/ui/alert-panel";
import type { FactoryImportActivationState } from "../hooks/use-factory-import-activation";
import type { FactoryImportPreviewState } from "../hooks/use-factory-import-preview";
import { allocateImportCreateFactoryName } from "../lib/allocate-import-create-factory-name";
import type {
  FactoryImportConfirmInput,
  FactoryImportSaveChoice,
} from "../lib/factory-import-save-choice";
import {
  getImportPreviewDialogMessages,
  IMPORT_PREVIEW_CURRENT_FACTORY_NAME_TOKEN,
} from "../messages/import-preview-dialog";

const IMPORT_SAVE_CHOICE_OPTION_CLASS =
  "grid cursor-pointer gap-1 rounded-xl border border-transparent p-3 transition-colors has-[:focus-visible]:border-primary has-[:checked]:border-primary has-[:checked]:bg-surface";

type ReadyFactoryImportPreviewState = Extract<
  FactoryImportPreviewState,
  { status: "ready" }
>;

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
      <span className="font-semibold text-on-surface">
        {currentFactoryName}
      </span>
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
    case "STALE_FACTORY_VERSION":
      return messages.errorByCode.STALE_FACTORY_VERSION;
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
    <AlertPanel
      aria-live="assertive"
      role="alert"
      tone="danger"
      variant="empty"
    >
      <div className="grid gap-1">
        <WidgetEmptyStateTitle>
          {messages.activationErrorTitle}
        </WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          {factoryImportActivationErrorCopy(error, locale)}
        </WidgetEmptyStateText>
      </div>
    </AlertPanel>
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
    <SurfacePanel asChild className="grid gap-3" radius="2xl" surface="low">
      <fieldset disabled={isSubmitting}>
        <Label as="legend" className="text-primary">
          {messages.saveChoiceLegend}
        </Label>
        <div
          className="grid gap-2"
          role="radiogroup"
          aria-label={messages.saveChoiceLegend}
        >
          <label
            className={IMPORT_SAVE_CHOICE_OPTION_CLASS}
            htmlFor={replaceOptionId}
          >
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
                <span className="text-base font-semibold text-on-surface">
                  {messages.replaceCurrentOption}
                </span>
                <Text as="span" className="m-0" variant="supporting">
                  {messages.replaceCurrentOptionDescription}
                </Text>
                <span className="text-sm font-semibold text-on-surface">
                  {currentSessionFactoryName}
                </span>
              </span>
            </span>
          </label>
          <label
            className={IMPORT_SAVE_CHOICE_OPTION_CLASS}
            htmlFor={createOptionId}
          >
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
                <span className="text-base font-semibold text-on-surface">
                  {messages.createNewNamedOption}
                </span>
                <Text as="span" className="m-0" variant="supporting">
                  {messages.createNewNamedOptionDescription}
                </Text>
                <span className="grid gap-1">
                  <Label as="span" className="text-primary">
                    {messages.createResolvedNameLabel}
                  </Label>
                  <span className="text-sm font-semibold text-on-surface">
                    {createFactoryName}
                  </span>
                </span>
              </span>
            </span>
          </label>
        </div>
      </fieldset>
    </SurfacePanel>
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
  const [choice, setChoice] =
    useState<FactoryImportSaveChoice>("replace_current");
  const resolvedExistingFactoryNames = useMemo(
    () =>
      collectExistingFactoryNames(
        currentSessionFactoryName,
        previewState.value.factory.name,
        existingFactoryNames,
      ),
    [
      currentSessionFactoryName,
      existingFactoryNames,
      previewState.value.factory.name,
    ],
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
        className="w-full max-w-5xl gap-6 p-4 md:grid-cols-[minmax(0,22rem)_minmax(0,1fr)] md:p-5"
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
        <SurfacePanel
          className="overflow-hidden"
          padding="default"
          radius="2xl"
          surface="low"
        >
          <img
            alt={messages.previewImageAlt(previewState.value.factory.name)}
            className="block h-full max-h-96 w-full rounded-2xl object-contain"
            src={previewState.value.previewImageSrc}
          />
        </SurfacePanel>
        <div className="grid content-start gap-5">
          <DialogHeader className="grid gap-3">
            <Label as="p" className="m-0 text-primary">
              {messages.flowLabel}
            </Label>
            <div className="grid gap-2">
              <Heading as={DialogTitle} className="m-0">
                {messages.title}
              </Heading>
              <Text as={DialogDescription} className="m-0">
                {renderImportPreviewCurrentFactoryDescription(
                  messages.descriptionTemplate,
                  currentSessionFactoryName,
                )}
              </Text>
            </div>
          </DialogHeader>

          <p className="m-0 text-base font-semibold text-on-surface">
            {previewState.value.factory.name}
          </p>

          <SurfacePanel asChild padding="default" radius="2xl" surface="low">
            <DescriptionList className="gap-3">
              <div className="grid gap-1">
                <Label as="dt" className="text-primary">
                  {messages.droppedFileLabel}
                </Label>
                <dd className="m-0 font-semibold text-on-surface">
                  {previewState.file.name}
                </dd>
              </div>
              <div className="grid gap-1">
                <Label as="dt" className="text-primary">
                  {messages.embeddedFactoryLabel}
                </Label>
                <dd className="m-0 font-semibold text-on-surface">
                  {previewState.value.factory.name}
                </dd>
              </div>
            </DescriptionList>
          </SurfacePanel>

          <FactoryImportSaveChoiceFieldset
            choice={choice}
            createFactoryName={createFactoryName}
            currentSessionFactoryName={currentSessionFactoryName}
            isSubmitting={isSubmitting}
            locale={locale}
            onChoiceChange={setChoice}
          />

          <Text className="m-0" variant="supporting">
            {messages.hint}
          </Text>

          {activationState.status === "error" ? (
            <FactoryImportActivationErrorPanel
              error={activationState.error}
              locale={locale}
            />
          ) : null}

          <DialogFooter>
            <Button
              disabled={isSubmitting}
              onClick={onCancel}
              tone="outline"
              type="button"
            >
              {messages.cancelAction}
            </Button>
            <Button
              aria-busy={isSubmitting ? "true" : undefined}
              disabled={isSubmitting}
              onClick={handleConfirm}
              type="button"
            >
              {isSubmitting
                ? messages.activatingAction
                : messages.activateAction}
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
