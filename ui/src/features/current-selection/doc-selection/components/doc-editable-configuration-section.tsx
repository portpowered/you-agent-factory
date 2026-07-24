import { useId } from "react";

import {
  CURRENT_SELECTION_FACTORY_DOC_MODEL_PATH,
  MonacoTextEditor,
} from "../../../../components/prompt-editor";
import { VerticalResizableWidth } from "../../../../components/prompt-editor/vertical-resizable-width";
import {
  AlertPanel,
  AlertPanelText,
  FormError,
  FormWarning,
  Input,
  Label,
  Text,
} from "../../../../components/ui";
import { FACTORY_DOCS_TARGET_PREFIX } from "../../../current-factory-definition/lib/doc-editable-values";
import { CurrentSelectionExpandableSection } from "../../base/components/detail/current-selection-expandable-section";
import { mergeDetailCardSaveFieldErrors } from "../../base/components/save/detail-card-factory-save-feedback";
import {
  CurrentSelectionDetailFeedback,
  CurrentSelectionFormField,
  CurrentSelectionSupportingText,
} from "../../base/public";
import { formatEditableDocOverwriteFieldLabels } from "../editing/editable-doc-overwrite-fields";
import type {
  DocDetailCardProps,
  EditableDocSaveState,
} from "../lib/detail-card-types";
import type { getDocDetailMessages } from "../messages/doc-detail";

export function DocEditableConfigurationSection({
  messages,
  saveState,
  state,
  targetPath,
}: {
  messages: ReturnType<typeof getDocDetailMessages>;
  saveState?: EditableDocSaveState;
  state: Extract<
    NonNullable<DocDetailCardProps["editableConfigurationState"]>,
    { status: "ready" }
  >;
  targetPath: string;
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;
  const fileNameErrorId = `${sectionId}-file-name-error`;
  const inlineContentErrorId = `${sectionId}-inline-content-error`;
  const validationErrors = mergeDetailCardSaveFieldErrors(
    state.validationErrors,
    saveState,
  );

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
      defaultExpanded
      headingId={headingId}
      title={messages.editableConfigurationHeading}
      toggleLabel={(expanded) =>
        expanded
          ? messages.editableConfigurationCollapseActionLabel
          : messages.editableConfigurationExpandActionLabel
      }
    >
      {state.overwriteFieldNames.length > 0 ? (
        <FormWarning>
          {messages.editableConfigurationOverwriteWarning(
            formatEditableDocOverwriteFieldLabels(
              state.overwriteFieldNames,
              messages,
            ),
          )}
          <Text className="m-0 text-on-surface-subtle" variant="supporting">
            {messages.editableConfigurationOverwriteWarningDetail}
          </Text>
        </FormWarning>
      ) : null}

      <CurrentSelectionFormField>
        <Label htmlFor={`${sectionId}-file-name`}>
          {messages.editableConfigurationFileNameFieldLabel}
        </Label>
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <CurrentSelectionSupportingText className="shrink-0">
            {FACTORY_DOCS_TARGET_PREFIX}
          </CurrentSelectionSupportingText>
          <Input
            aria-describedby={
              validationErrors.fileName ? fileNameErrorId : undefined
            }
            aria-invalid={Boolean(validationErrors.fileName)}
            className="min-w-0 flex-1"
            id={`${sectionId}-file-name`}
            onChange={(event) => state.onFileNameChange(event.target.value)}
            value={state.draft.fileName}
          />
        </div>
        {validationErrors.fileName ? (
          <FormError id={fileNameErrorId}>
            {validationErrors.fileName}
          </FormError>
        ) : (
          <CurrentSelectionSupportingText>
            {messages.editableConfigurationTargetPathPrefix}: {targetPath}
          </CurrentSelectionSupportingText>
        )}
      </CurrentSelectionFormField>

      <CurrentSelectionFormField>
        <Label htmlFor={`${sectionId}-inline-content`}>
          {messages.editableConfigurationInlineContentFieldLabel}
        </Label>
        <VerticalResizableWidth resizeHandleLabel={messages.docKindLabel}>
          <MonacoTextEditor
            ariaDescribedBy={
              validationErrors.inlineContent ? inlineContentErrorId : undefined
            }
            ariaInvalid={Boolean(validationErrors.inlineContent)}
            ariaLabel={messages.editableConfigurationInlineContentFieldLabel}
            hasError={Boolean(validationErrors.inlineContent)}
            height="100%"
            id={`${sectionId}-inline-content`}
            loadingMessage={messages.editableConfigurationEditorLoading}
            modelPath={`${CURRENT_SELECTION_FACTORY_DOC_MODEL_PATH}/${targetPath}`}
            onChange={state.onInlineContentChange}
            startupErrorMessage={messages.editableConfigurationEditorError}
            value={state.draft.inlineContent}
          />
        </VerticalResizableWidth>
        {validationErrors.inlineContent ? (
          <FormError id={inlineContentErrorId}>
            {validationErrors.inlineContent}
          </FormError>
        ) : null}
      </CurrentSelectionFormField>

      {state.hasValidationErrors ? (
        <AlertPanel role="status" tone="warning">
          <AlertPanelText>
            {messages.editableConfigurationValidationStatus}
          </AlertPanelText>
          <AlertPanelText
            className="text-on-surface-subtle"
            variant="supporting"
          >
            {messages.editableConfigurationSaveDisabledValidationDetail}
          </AlertPanelText>
        </AlertPanel>
      ) : null}

      {saveState?.status === "error" && !saveState.fieldErrors ? (
        <CurrentSelectionDetailFeedback role="alert" tone="danger">
          {messages.editableConfigurationSaveErrorPrefix}{" "}
          {saveState.errorMessage}
        </CurrentSelectionDetailFeedback>
      ) : null}
      {saveState?.status === "warning" ? (
        <AlertPanel role="alert" tone="warning">
          <AlertPanelText>{saveState.message}</AlertPanelText>
          <AlertPanelText
            className="text-on-surface-subtle"
            variant="supporting"
          >
            {messages.editableConfigurationSaveStaleVersionDetail}
          </AlertPanelText>
        </AlertPanel>
      ) : null}
    </CurrentSelectionExpandableSection>
  );
}
