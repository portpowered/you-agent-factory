import { useEffect, useId, useRef, useState } from "react";

import type { FactoryValue } from "../../api/named-factory";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard";
import { cx } from "../../components/dashboard/classnames";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
} from "../../components/ui";
import { buildFactoryExportFilename } from "./build-factory-export-filename";
import { downloadBlobAsFile } from "./browser-download";
import type { CurrentFactoryExportFailure } from "./use-current-factory-export";
import { writeFactoryExportPng } from "./factory-png-export";

const DIALOG_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);
const DIALOG_BODY_CLASS = cx("m-0 max-w-lg", DASHBOARD_BODY_TEXT_CLASS);
const DIALOG_HINT_CLASS = cx("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS);
const DIALOG_FORM_CLASS = "space-y-5";
const DIALOG_FIELD_GROUP_CLASS = "space-y-2";
const DIALOG_FIELD_LABEL_CLASS = cx(
  "block text-sm font-semibold text-af-ink",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const DIALOG_FILE_INPUT_CLASS =
  "block w-full rounded-xl border border-dashed border-af-overlay/18 bg-af-overlay/4 px-3 py-3 text-sm text-af-ink/80 file:mr-3 file:rounded-lg file:border-0 file:bg-af-accent/12 file:px-3 file:py-2 file:text-sm file:font-semibold file:text-af-accent hover:bg-af-overlay/6";
const DIALOG_FIELD_DESCRIPTION_CLASS = cx("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS);
const DIALOG_VALIDATION_CLASS = "m-0 text-sm font-medium text-af-danger-ink";
const DIALOG_ERROR_PANEL_CLASS =
  "rounded-2xl border border-af-danger/30 bg-af-danger/10 px-4 py-3 text-sm text-af-danger-ink";
const DIALOG_CONTENT_CLASS = "w-[min(92vw,42rem)] gap-6";

export interface ExportFactoryDialogProps {
  factory: FactoryValue | null;
  initialFactoryName: string;
  isPreparing?: boolean;
  isOpen: boolean;
  onClose: () => void;
  preparationFailure?: CurrentFactoryExportFailure | null;
}

type ExportDialogState =
  | { status: "idle" }
  | { status: "error"; message: string }
  | { status: "exporting" };

interface ExportDialogFormState {
  dialogState: ExportDialogState;
  exportDisabled: boolean;
  exportName: string;
  handleClose: () => void;
  handleExport: () => Promise<void>;
  handleImageSelection: (files: FileList | null) => void;
  handleOpenChange: (open: boolean) => void;
  imageTouched: boolean;
  imageValidationId: string;
  imageValidationMessage: string | null;
  isExporting: boolean;
  nameTouched: boolean;
  nameValidationId: string;
  nameValidationMessage: string | null;
  selectedImage: File | null;
  setDialogState: (state: ExportDialogState) => void;
  setExportName: (value: string) => void;
  setImageTouched: (value: boolean) => void;
  setNameTouched: (value: boolean) => void;
}

export function ExportFactoryDialog({
  factory,
  initialFactoryName,
  isPreparing = false,
  isOpen,
  onClose,
  preparationFailure = null,
}: ExportFactoryDialogProps) {
  const validationIdBase = useId();
  const formState = useExportFactoryDialogState({
    factory,
    initialFactoryName,
    isOpen,
    onClose,
    preparationFailure,
    validationIdBase,
  });

  if (!isOpen) {
    return null;
  }

  return (
    <Dialog onOpenChange={formState.handleOpenChange} open={isOpen}>
      <DialogContent className={DIALOG_CONTENT_CLASS}>
        <DialogHeader>
          <div className="space-y-2">
            <DialogTitle className={DIALOG_TITLE_CLASS}>
              Export factory
            </DialogTitle>
            <DialogDescription className={DIALOG_BODY_CLASS}>
              Package the current factory into a PNG artifact without changing the live
              dashboard state.
            </DialogDescription>
          </div>
        </DialogHeader>

        <p className={DIALOG_HINT_CLASS}>
          Confirming export keeps the current dashboard state unchanged and downloads
          a PNG artifact with embedded Port OS factory metadata.
        </p>

        <ExportFactoryDialogForm formState={formState} />
        <ExportFactoryDialogMessages
          dialogState={formState.dialogState}
          factory={factory}
          isPreparing={isPreparing}
          preparationFailure={preparationFailure}
        />

        <DialogFooter>
          <Button onClick={formState.handleClose} tone="outline" type="button">
            Cancel
          </Button>
          <Button
            aria-busy={formState.isExporting ? "true" : undefined}
            disabled={formState.exportDisabled || isPreparing}
            onClick={() => {
              void formState.handleExport();
            }}
            type="button"
          >
            {formState.dialogState.status === "exporting" ? "Exporting..." : "Export PNG"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ExportFactoryDialogForm({
  formState,
}: {
  formState: ExportDialogFormState;
}) {
  return (
    <div className={DIALOG_FORM_CLASS}>
      <div className={DIALOG_FIELD_GROUP_CLASS}>
        <label className={DIALOG_FIELD_LABEL_CLASS} htmlFor="export-factory-name">
          Factory name
        </label>
        <Input
          aria-describedby={
            formState.nameValidationMessage ? formState.nameValidationId : undefined
          }
          aria-invalid={formState.nameValidationMessage ? "true" : undefined}
          className={DASHBOARD_BODY_TEXT_CLASS}
          disabled={formState.isExporting}
          id="export-factory-name"
          onBlur={() => {
            formState.setNameTouched(true);
          }}
          onChange={(event) => {
            if (formState.isExporting) {
              return;
            }
            formState.setDialogState({ status: "idle" });
            formState.setExportName(event.target.value);
          }}
          placeholder="Factory name"
          type="text"
          value={formState.exportName}
        />
        <p className={DIALOG_FIELD_DESCRIPTION_CLASS}>
          This name is embedded in the exported PNG metadata and used for the
          downloaded filename.
        </p>
        {formState.nameValidationMessage ? (
          <p className={DIALOG_VALIDATION_CLASS} id={formState.nameValidationId}>
            {formState.nameValidationMessage}
          </p>
        ) : null}
      </div>

      <div className={DIALOG_FIELD_GROUP_CLASS}>
        <label className={DIALOG_FIELD_LABEL_CLASS} htmlFor="export-factory-image">
          Cover image
        </label>
        <input
          accept="image/*"
          aria-describedby={
            formState.imageValidationMessage ? formState.imageValidationId : undefined
          }
          aria-invalid={formState.imageValidationMessage ? "true" : undefined}
          className={DIALOG_FILE_INPUT_CLASS}
          disabled={formState.isExporting}
          id="export-factory-image"
          onChange={(event) => {
            if (formState.isExporting) {
              return;
            }
            formState.setDialogState({ status: "idle" });
            formState.handleImageSelection(event.target.files);
          }}
          type="file"
        />
        <p className={DIALOG_FIELD_DESCRIPTION_CLASS}>
          Choose the image customers will see when they open the exported PNG.
        </p>
        {formState.selectedImage ? (
          <p className={DIALOG_HINT_CLASS}>Selected image: {formState.selectedImage.name}</p>
        ) : null}
        {formState.imageValidationMessage ? (
          <p className={DIALOG_VALIDATION_CLASS} id={formState.imageValidationId}>
            {formState.imageValidationMessage}
          </p>
        ) : null}
      </div>
    </div>
  );
}

function ExportFactoryDialogMessages({
  dialogState,
  factory,
  isPreparing,
  preparationFailure,
}: Pick<ExportDialogFormState, "dialogState"> & {
  factory: FactoryValue | null;
  isPreparing: boolean;
  preparationFailure?: CurrentFactoryExportFailure | null;
}) {
  return (
    <>
      {isPreparing ? (
        <div className={DIALOG_ERROR_PANEL_CLASS} role="status">
          Loading the current authored factory definition.
        </div>
      ) : null}

      {preparationFailure && factory === null && !isPreparing ? (
        <div className={DIALOG_ERROR_PANEL_CLASS} role="status">
          {preparationFailure.message}
        </div>
      ) : null}

      {dialogState.status === "error" ? (
        <div className={DIALOG_ERROR_PANEL_CLASS} role="alert">
          {dialogState.message}
        </div>
      ) : null}
    </>
  );
}

function useExportFactoryDialogState({
  factory,
  initialFactoryName,
  isOpen,
  onClose,
  preparationFailure,
  validationIdBase,
}: {
  factory: FactoryValue | null;
  initialFactoryName: string;
  isOpen: boolean;
  onClose: () => void;
  preparationFailure?: CurrentFactoryExportFailure | null;
  validationIdBase: string;
}): ExportDialogFormState {
  const [exportName, setExportName] = useState(initialFactoryName);
  const [selectedImage, setSelectedImage] = useState<File | null>(null);
  const [imageSelectionError, setImageSelectionError] = useState<string | null>(null);
  const [nameTouched, setNameTouched] = useState(false);
  const [imageTouched, setImageTouched] = useState(false);
  const [dialogState, setDialogState] = useState<ExportDialogState>({ status: "idle" });
  const exportAttemptRef = useRef(0);
  const trimmedExportName = exportName.trim();
  const nameValidationMessage =
    nameTouched && trimmedExportName.length === 0
      ? "Enter a factory name before exporting."
      : null;
  const imageValidationMessage = imageSelectionError
    ? imageSelectionError
    : imageTouched && !selectedImage
      ? "Choose a cover image before exporting."
      : null;
  const nameValidationId = `${validationIdBase}-name-validation`;
  const imageValidationId = `${validationIdBase}-image-validation`;
  const isExporting = dialogState.status === "exporting";
  const exportDisabled = isExporting || factory === null;

  const handleClose = () => {
    exportAttemptRef.current += 1;
    onClose();
  };

  useResetExportFactoryDialogState({
    exportAttemptRef,
    initialFactoryName,
    isOpen,
    setDialogState,
    setExportName,
    setImageSelectionError,
    setImageTouched,
    setNameTouched,
    setSelectedImage,
  });

  const handleImageSelection = (files: FileList | null) => {
    setImageTouched(true);

    const selectedFile = files?.item?.(0) ?? files?.[0] ?? null;
    if (!selectedFile) {
      setSelectedImage(null);
      setImageSelectionError("Choose a cover image before exporting.");
      return;
    }

    if (selectedFile.type && !selectedFile.type.startsWith("image/")) {
      setSelectedImage(null);
      setImageSelectionError("Choose an image file before exporting.");
      return;
    }

    setSelectedImage(selectedFile);
    setImageSelectionError(null);
  };

  const handleExport = async () => {
    setNameTouched(true);
    setImageTouched(true);

    if (!factory) {
      setDialogState({
        message:
          preparationFailure?.message ??
          "The current factory definition is not available for export yet.",
        status: "error",
      });
      return;
    }

    if (!selectedImage || trimmedExportName.length === 0) {
      return;
    }

    const exportAttempt = exportAttemptRef.current + 1;
    exportAttemptRef.current = exportAttempt;
    setDialogState({ status: "exporting" });

    const result = await writeFactoryExportPng({
      factory: {
        ...factory,
        name: trimmedExportName,
      },
      image: selectedImage,
    });

    if (exportAttemptRef.current !== exportAttempt) {
      return;
    }

    if (!result.ok) {
      setDialogState({
        message: result.error.message,
        status: "error",
      });
      return;
    }

    downloadBlobAsFile({
      blob: result.blob,
      filename: buildFactoryExportFilename(trimmedExportName),
    });
    handleClose();
  };

  return {
    dialogState,
    exportDisabled,
    exportName,
    handleClose,
    handleOpenChange: createHandleOpenChange(handleClose),
    handleExport,
    handleImageSelection,
    imageTouched,
    imageValidationId,
    imageValidationMessage,
    isExporting,
    nameTouched,
    nameValidationId,
    nameValidationMessage,
    selectedImage,
    setDialogState,
    setExportName,
    setImageTouched,
    setNameTouched,
  };
}

function createHandleOpenChange(handleClose: () => void) {
  return (open: boolean) => {
    if (!open) {
      handleClose();
    }
  };
}

function useResetExportFactoryDialogState({
  exportAttemptRef,
  initialFactoryName,
  isOpen,
  setDialogState,
  setExportName,
  setImageSelectionError,
  setImageTouched,
  setNameTouched,
  setSelectedImage,
}: {
  exportAttemptRef: React.RefObject<number>;
  initialFactoryName: string;
  isOpen: boolean;
  setDialogState: (state: ExportDialogState) => void;
  setExportName: (value: string) => void;
  setImageSelectionError: (value: string | null) => void;
  setImageTouched: (value: boolean) => void;
  setNameTouched: (value: boolean) => void;
  setSelectedImage: (value: File | null) => void;
}) {
  useEffect(() => {
    if (!isOpen) {
      exportAttemptRef.current += 1;
      return;
    }

    setDialogState({ status: "idle" });
    setExportName(initialFactoryName);
    setSelectedImage(null);
    setImageSelectionError(null);
    setImageTouched(false);
    setNameTouched(false);
  }, [
    exportAttemptRef,
    initialFactoryName,
    isOpen,
    setDialogState,
    setExportName,
    setImageSelectionError,
    setImageTouched,
    setNameTouched,
    setSelectedImage,
  ]);
}
