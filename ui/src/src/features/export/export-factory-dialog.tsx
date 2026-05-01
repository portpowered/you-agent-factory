import { useEffect, useRef, useState } from "react";

import type { NamedFactoryValue } from "../../../api/named-factory";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard";
import { cx } from "../../components/dashboard/classnames";
import { buildFactoryExportFilename } from "./build-factory-export-filename";
import { downloadBlobAsFile } from "./browser-download";
import type { CurrentFactoryExportFailure } from "./use-current-factory-export";
import { writeFactoryExportPng } from "./factory-png-export";

const DIALOG_BACKDROP_CLASS =
  "fixed inset-0 z-50 flex items-center justify-center bg-af-canvas/78 px-5 py-6 backdrop-blur-sm";
const DIALOG_PANEL_CLASS =
  "w-full max-w-xl rounded-[1.75rem] border border-af-overlay/12 bg-af-surface/96 p-6 shadow-af-panel";
const DIALOG_HEADER_CLASS = "mb-4 flex items-start justify-between gap-4";
const DIALOG_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);
const DIALOG_BODY_CLASS = cx("m-0 max-w-lg", DASHBOARD_BODY_TEXT_CLASS);
const DIALOG_HINT_CLASS = cx("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS);
const DIALOG_FORM_CLASS = "mt-6 space-y-5";
const DIALOG_FIELD_GROUP_CLASS = "space-y-2";
const DIALOG_FIELD_LABEL_CLASS = cx(
  "block text-sm font-semibold text-af-ink",
  DASHBOARD_SUPPORTING_LABELS_CLASS,
);
const DIALOG_FIELD_INPUT_CLASS =
  "w-full rounded-xl border border-af-overlay/14 bg-af-canvas/78 px-3 py-2.5 text-sm text-af-ink outline-af-accent transition placeholder:text-af-ink/42 focus-visible:outline-2 focus-visible:outline-offset-2";
const DIALOG_FILE_INPUT_CLASS =
  "block w-full rounded-xl border border-dashed border-af-overlay/18 bg-af-overlay/4 px-3 py-3 text-sm text-af-ink/80 file:mr-3 file:rounded-lg file:border-0 file:bg-af-accent/12 file:px-3 file:py-2 file:text-sm file:font-semibold file:text-af-accent hover:bg-af-overlay/6";
const DIALOG_FIELD_DESCRIPTION_CLASS = cx("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS);
const DIALOG_VALIDATION_CLASS = "m-0 text-sm font-medium text-af-danger-ink";
const DIALOG_ERROR_PANEL_CLASS =
  "rounded-2xl border border-af-danger/30 bg-af-danger/10 px-4 py-3 text-sm text-af-danger-ink";
const DIALOG_CLOSE_BUTTON_CLASS =
  "inline-flex h-10 w-10 items-center justify-center rounded-full border border-af-overlay/12 bg-af-overlay/4 text-af-ink/72 outline-af-accent transition hover:bg-af-overlay/10 hover:text-af-ink focus-visible:outline-2 focus-visible:outline-offset-2";
const DIALOG_ACTION_ROW_CLASS = "mt-6 flex flex-wrap items-center justify-end gap-3";
const DIALOG_CANCEL_BUTTON_CLASS =
  "rounded-lg border border-af-overlay/14 bg-af-overlay/6 px-4 py-2 text-sm font-semibold text-af-ink/78 outline-af-accent transition hover:bg-af-overlay/10 hover:text-af-ink focus-visible:outline-2 focus-visible:outline-offset-2";
const DIALOG_EXPORT_BUTTON_CLASS =
  "rounded-lg bg-af-accent px-4 py-2 text-sm font-semibold text-af-accent-contrast outline-af-accent transition hover:bg-af-accent/90 focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:bg-af-overlay/18 disabled:text-af-ink/48";

export interface ExportFactoryDialogProps {
  namedFactory: NamedFactoryValue | null;
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

export function ExportFactoryDialog({
  namedFactory,
  initialFactoryName,
  isPreparing = false,
  isOpen,
  onClose,
  preparationFailure = null,
}: ExportFactoryDialogProps) {
  const [exportName, setExportName] = useState(initialFactoryName);
  const [selectedImage, setSelectedImage] = useState<File | null>(null);
  const [imageSelectionError, setImageSelectionError] = useState<string | null>(null);
  const [nameTouched, setNameTouched] = useState(false);
  const [imageTouched, setImageTouched] = useState(false);
  const [dialogState, setDialogState] = useState<ExportDialogState>({ status: "idle" });
  const exportAttemptRef = useRef(0);

  const handleClose = () => {
    exportAttemptRef.current += 1;
    onClose();
  };

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        handleClose();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [handleClose, isOpen]);

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
  }, [initialFactoryName, isOpen]);

  if (!isOpen) {
    return null;
  }

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
  const isExporting = dialogState.status === "exporting";
  const exportDisabled = isExporting || isPreparing || namedFactory === null;

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

    if (!namedFactory) {
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
      image: selectedImage,
      namedFactory: {
        ...namedFactory,
        name: trimmedExportName,
      },
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

  return (
    <div className={DIALOG_BACKDROP_CLASS} onClick={handleClose}>
      <section
        aria-describedby="export-factory-dialog-description"
        aria-labelledby="export-factory-dialog-title"
        aria-modal="true"
        className={DIALOG_PANEL_CLASS}
        onClick={(event) => {
          event.stopPropagation();
        }}
        role="dialog"
      >
        <header className={DIALOG_HEADER_CLASS}>
          <div className="space-y-2">
            <h2 className={DIALOG_TITLE_CLASS} id="export-factory-dialog-title">
              Export factory
            </h2>
            <p className={DIALOG_BODY_CLASS} id="export-factory-dialog-description">
              Package the current factory into a PNG artifact without changing the live
              dashboard state.
            </p>
          </div>

          <button
            aria-label="Close export dialog"
            className={DIALOG_CLOSE_BUTTON_CLASS}
            onClick={handleClose}
            type="button"
          >
            <svg
              aria-hidden="true"
              fill="none"
              height="18"
              stroke="currentColor"
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth="1.8"
              viewBox="0 0 24 24"
              width="18"
            >
              <path d="M6 6l12 12" />
              <path d="M18 6L6 18" />
            </svg>
          </button>
        </header>

        <p className={DIALOG_HINT_CLASS}>
          Confirming export keeps the current dashboard state unchanged and downloads
          a PNG artifact with embedded Port OS factory metadata.
        </p>

        <div className={DIALOG_FORM_CLASS}>
          <div className={DIALOG_FIELD_GROUP_CLASS}>
            <label className={DIALOG_FIELD_LABEL_CLASS} htmlFor="export-factory-name">
              Factory name
            </label>
            <input
              className={DIALOG_FIELD_INPUT_CLASS}
              disabled={isExporting}
              id="export-factory-name"
              onBlur={() => {
                setNameTouched(true);
              }}
              onChange={(event) => {
                if (isExporting) {
                  return;
                }
                setDialogState({ status: "idle" });
                setExportName(event.target.value);
              }}
              placeholder="Factory name"
              type="text"
              value={exportName}
            />
            <p className={DIALOG_FIELD_DESCRIPTION_CLASS}>
              This name is embedded in the exported PNG metadata and used for the
              downloaded filename.
            </p>
            {nameValidationMessage ? (
              <p className={DIALOG_VALIDATION_CLASS}>{nameValidationMessage}</p>
            ) : null}
          </div>

          <div className={DIALOG_FIELD_GROUP_CLASS}>
            <label className={DIALOG_FIELD_LABEL_CLASS} htmlFor="export-factory-image">
              Cover image
            </label>
            <input
              accept="image/*"
              className={DIALOG_FILE_INPUT_CLASS}
              disabled={isExporting}
              id="export-factory-image"
              onChange={(event) => {
                if (isExporting) {
                  return;
                }
                setDialogState({ status: "idle" });
                handleImageSelection(event.target.files);
              }}
              type="file"
            />
            <p className={DIALOG_FIELD_DESCRIPTION_CLASS}>
              Choose the image customers will see when they open the exported PNG.
            </p>
            {selectedImage ? (
              <p className={DIALOG_HINT_CLASS}>Selected image: {selectedImage.name}</p>
            ) : null}
            {imageValidationMessage ? (
              <p className={DIALOG_VALIDATION_CLASS}>{imageValidationMessage}</p>
            ) : null}
          </div>

          {isPreparing ? (
            <div className={DIALOG_ERROR_PANEL_CLASS} role="status">
              Loading the current authored factory definition.
            </div>
          ) : null}

          {preparationFailure && namedFactory === null && !isPreparing ? (
            <div className={DIALOG_ERROR_PANEL_CLASS} role="status">
              {preparationFailure.message}
            </div>
          ) : null}

          {dialogState.status === "error" ? (
            <div className={DIALOG_ERROR_PANEL_CLASS} role="alert">
              {dialogState.message}
            </div>
          ) : null}
        </div>

        <div className={DIALOG_ACTION_ROW_CLASS}>
          <button
            className={DIALOG_CANCEL_BUTTON_CLASS}
            onClick={handleClose}
            type="button"
          >
            Cancel
          </button>
          <button
            className={DIALOG_EXPORT_BUTTON_CLASS}
            disabled={exportDisabled}
            onClick={() => {
              void handleExport();
            }}
            type="button"
          >
            {dialogState.status === "exporting" ? "Exporting..." : "Export PNG"}
          </button>
        </div>
      </section>
    </div>
  );
}
