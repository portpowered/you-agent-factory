import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@you-agent-factory/components/overlays";
import { useEffect, useId, useRef, useState } from "react";
import type { ImportFactoryValue } from "../../../api/session-factory";
import { Button, Text } from "@you-agent-factory/components/primitives";
import { AlertPanel, AlertPanelText } from "../../../components/ui/alert-panel";
import {
  FormDescription,
  FormError,
  FormField,
  FormLabel,
} from "../../../components/ui/form-field";
import { Input } from "../../../components/ui/input";
import type { CurrentFactoryExportFailure } from "../hooks/use-current-factory-export";
import { downloadBlobAsFile } from "../lib/browser-download";
import { buildFactoryExportFilename } from "../lib/build-factory-export-filename";
import { writeFactoryExportPng } from "../lib/factory-png-export";
import { getExportDialogMessages } from "../messages/export-dialog";
import { ExportFactoryDialogImageField } from "./export-factory-dialog-image-field";

export interface ExportFactoryDialogProps {
  factory: ImportFactoryValue | null;
  initialFactoryName: string;
  isPreparing?: boolean;
  isOpen: boolean;
  locale?: string;
  onClose: () => void;
  preparationFailure?: CurrentFactoryExportFailure | null;
}

type ExportDialogState =
  | { status: "idle" }
  | { status: "error"; message: string }
  | { status: "exporting" }
  | { status: "success"; filename: string };

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
  locale,
  onClose,
  preparationFailure = null,
}: ExportFactoryDialogProps) {
  const messages = getExportDialogMessages(locale);
  const validationIdBase = useId();
  const formState = useExportFactoryDialogState({
    factory,
    initialFactoryName,
    isOpen,
    locale,
    messages,
    onClose,
    preparationFailure,
    validationIdBase,
  });

  if (!isOpen) {
    return null;
  }

  return (
    <Dialog onOpenChange={formState.handleOpenChange} open={isOpen}>
      <DialogContent
        className="w-full max-w-2xl gap-6"
        closeLabel={messages.closeLabel}
      >
        <DialogHeader>
          <div className="space-y-2">
            <DialogTitle className="m-0">{messages.title}</DialogTitle>
            <DialogDescription className="m-0 max-w-lg">
              {messages.description}
            </DialogDescription>
          </div>
        </DialogHeader>

        <Text className="m-0" variant="supporting">
          {messages.hint}
        </Text>

        <ExportFactoryDialogForm formState={formState} messages={messages} />
        <ExportFactoryDialogMessages
          dialogState={formState.dialogState}
          factory={factory}
          isPreparing={isPreparing}
          messages={messages}
          preparationFailure={preparationFailure}
        />

        <DialogFooter>
          <Button onClick={formState.handleClose} tone="outline" type="button">
            {formState.dialogState.status === "success"
              ? messages.closeAction
              : messages.cancelAction}
          </Button>
          <Button
            aria-busy={formState.isExporting ? "true" : undefined}
            disabled={formState.exportDisabled || isPreparing}
            onClick={() => {
              void formState.handleExport();
            }}
            type="button"
          >
            {formState.dialogState.status === "exporting"
              ? messages.exportingAction
              : messages.exportAction}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ExportFactoryDialogForm({
  formState,
  messages,
}: {
  formState: ExportDialogFormState;
  messages: ReturnType<typeof getExportDialogMessages>;
}) {
  return (
    <div className="space-y-5">
      <FormField>
        <FormLabel htmlFor="export-factory-name">
          {messages.nameLabel}
        </FormLabel>
        <Input
          aria-describedby={
            formState.nameValidationMessage
              ? formState.nameValidationId
              : undefined
          }
          aria-invalid={formState.nameValidationMessage ? "true" : undefined}
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
          placeholder={messages.namePlaceholder}
          type="text"
          value={formState.exportName}
        />
        <FormDescription>{messages.nameDescription}</FormDescription>
        {formState.nameValidationMessage ? (
          <FormError id={formState.nameValidationId}>
            {formState.nameValidationMessage}
          </FormError>
        ) : null}
      </FormField>

      <ExportFactoryDialogImageField
        imageDescription={messages.imageDescription}
        imageLabel={messages.imageLabel}
        imageValidationId={formState.imageValidationId}
        imageValidationMessage={formState.imageValidationMessage}
        isExporting={formState.isExporting}
        onImageChange={formState.handleImageSelection}
        onInteraction={() => {
          formState.setDialogState({ status: "idle" });
        }}
        selectedImage={formState.selectedImage}
        selectedImageLabel={messages.selectedImageLabel}
      />
    </div>
  );
}

function ExportFactoryDialogMessages({
  dialogState,
  factory,
  isPreparing,
  messages,
  preparationFailure,
}: Pick<ExportDialogFormState, "dialogState"> & {
  factory: ImportFactoryValue | null;
  isPreparing: boolean;
  messages: ReturnType<typeof getExportDialogMessages>;
  preparationFailure?: CurrentFactoryExportFailure | null;
}) {
  return (
    <>
      {isPreparing ? (
        <AlertPanel role="status" tone="danger">
          <AlertPanelText>{messages.loadingStatus}</AlertPanelText>
        </AlertPanel>
      ) : null}

      {preparationFailure && factory === null && !isPreparing ? (
        <AlertPanel role="status" tone="danger">
          <AlertPanelText>{preparationFailure.message}</AlertPanelText>
        </AlertPanel>
      ) : null}

      {dialogState.status === "error" ? (
        <AlertPanel role="alert" tone="danger">
          <AlertPanelText>{dialogState.message}</AlertPanelText>
        </AlertPanel>
      ) : null}

      {dialogState.status === "success" ? (
        <AlertPanel aria-live="polite" role="status" tone="success">
          <AlertPanelText>
            {messages.successMessage(dialogState.filename)}
          </AlertPanelText>
        </AlertPanel>
      ) : null}
    </>
  );
}

function useExportFactoryDialogState({
  factory,
  initialFactoryName,
  isOpen,
  locale,
  messages,
  onClose,
  preparationFailure,
  validationIdBase,
}: {
  factory: ImportFactoryValue | null;
  initialFactoryName: string;
  isOpen: boolean;
  locale?: string | null;
  messages: ReturnType<typeof getExportDialogMessages>;
  onClose: () => void;
  preparationFailure?: CurrentFactoryExportFailure | null;
  validationIdBase: string;
}): ExportDialogFormState {
  const [exportName, setExportName] = useState(initialFactoryName);
  const [selectedImage, setSelectedImage] = useState<File | null>(null);
  const [imageSelectionError, setImageSelectionError] = useState<string | null>(
    null,
  );
  const [nameTouched, setNameTouched] = useState(false);
  const [imageTouched, setImageTouched] = useState(false);
  const [dialogState, setDialogState] = useState<ExportDialogState>({
    status: "idle",
  });
  const exportAttemptRef = useRef(0);
  const trimmedExportName = exportName.trim();
  const nameValidationMessage =
    nameTouched && trimmedExportName.length === 0
      ? messages.nameRequiredValidation
      : null;
  const imageValidationMessage = imageSelectionError
    ? imageSelectionError
    : imageTouched && !selectedImage
      ? messages.imageRequiredValidation
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
    exportName,
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

  const handleExport = async () => {
    setNameTouched(true);
    setImageTouched(true);

    if (!factory) {
      setDialogState({
        message: preparationFailure?.message ?? messages.exportUnavailable,
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
      ...(locale && locale !== "en" ? { locale } : {}),
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

    const filename = buildFactoryExportFilename(trimmedExportName);
    downloadBlobAsFile({
      blob: result.blob,
      filename,
    });
    setDialogState({
      filename,
      status: "success",
    });
  };

  return {
    dialogState,
    exportDisabled,
    exportName,
    handleClose,
    handleOpenChange: createHandleOpenChange(handleClose),
    handleExport,
    handleImageSelection: createHandleImageSelection({
      setImageSelectionError,
      setImageTouched,
      setSelectedImage,
      messages,
    }),
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

function createHandleImageSelection({
  messages,
  setImageSelectionError,
  setImageTouched,
  setSelectedImage,
}: {
  messages: ReturnType<typeof getExportDialogMessages>;
  setImageSelectionError: (value: string | null) => void;
  setImageTouched: (value: boolean) => void;
  setSelectedImage: (value: File | null) => void;
}) {
  return (files: FileList | null) => {
    setImageTouched(true);
    const selectedFile = files?.item?.(0) ?? files?.[0] ?? null;
    if (!selectedFile) {
      setSelectedImage(null);
      setImageSelectionError(messages.imageRequiredValidation);
      return;
    }

    if (selectedFile.type && !selectedFile.type.startsWith("image/")) {
      setSelectedImage(null);
      setImageSelectionError(messages.imageTypeValidation);
      return;
    }

    setSelectedImage(selectedFile);
    setImageSelectionError(null);
  };
}

function createHandleOpenChange(handleClose: () => void) {
  return (open: boolean) => !open && handleClose();
}

function useResetExportFactoryDialogState({
  exportName,
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
  exportName: string;
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
  const previousInitialFactoryNameRef = useRef(initialFactoryName);
  const wasOpenRef = useRef(false);

  useEffect(() => {
    if (!isOpen) {
      exportAttemptRef.current += 1;
      wasOpenRef.current = false;
      previousInitialFactoryNameRef.current = initialFactoryName;
      return;
    }

    const previousInitialFactoryName = previousInitialFactoryNameRef.current;
    const isOpening = !wasOpenRef.current;

    if (isOpening) {
      setDialogState({ status: "idle" });
      setExportName(initialFactoryName);
      setSelectedImage(null);
      setImageSelectionError(null);
      setImageTouched(false);
      setNameTouched(false);
      wasOpenRef.current = true;
      previousInitialFactoryNameRef.current = initialFactoryName;
      return;
    }

    if (exportName === previousInitialFactoryName) {
      setExportName(initialFactoryName);
    }
    previousInitialFactoryNameRef.current = initialFactoryName;
  }, [
    exportName,
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
