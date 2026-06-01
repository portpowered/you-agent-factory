import {
  DASHBOARD_SUPPORTING_LABELS_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import {
  CHOOSE_FILE_NATIVE_INPUT_CLASS,
  ChooseFileField,
} from "../../choose-file/public";

const IMAGE_FIELD_HINT_CLASS = cn("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS);
const IMAGE_FIELD_VALIDATION_CLASS =
  "m-0 text-sm font-medium text-af-danger-text";

export interface ExportFactoryDialogImageFieldProps {
  imageDescription: string;
  imageLabel: string;
  imageValidationId: string;
  imageValidationMessage: string | null;
  isExporting: boolean;
  onImageChange: (files: FileList | null) => void;
  onInteraction: () => void;
  selectedImage: File | null;
  selectedImageLabel: (filename: string) => string;
}

export function ExportFactoryDialogImageField({
  imageDescription,
  imageLabel,
  imageValidationId,
  imageValidationMessage,
  isExporting,
  onImageChange,
  onInteraction,
  selectedImage,
  selectedImageLabel,
}: ExportFactoryDialogImageFieldProps) {
  return (
    <ChooseFileField
      afterControl={
        <>
          {selectedImage ? (
            <p className={IMAGE_FIELD_HINT_CLASS}>
              {selectedImageLabel(selectedImage.name)}
            </p>
          ) : null}
          {imageValidationMessage ? (
            <p className={IMAGE_FIELD_VALIDATION_CLASS} id={imageValidationId}>
              {imageValidationMessage}
            </p>
          ) : null}
        </>
      }
      control={
        <input
          accept="image/*"
          aria-describedby={
            imageValidationMessage ? imageValidationId : undefined
          }
          aria-invalid={imageValidationMessage ? "true" : undefined}
          className={CHOOSE_FILE_NATIVE_INPUT_CLASS}
          disabled={isExporting}
          id="export-factory-image"
          onChange={(event) => {
            if (isExporting) {
              return;
            }
            onInteraction();
            onImageChange(event.target.files);
          }}
          type="file"
        />
      }
      description={
        <p className={cn("m-0", DASHBOARD_SUPPORTING_TEXT_CLASS)}>
          {imageDescription}
        </p>
      }
      disabled={isExporting}
      label={
        <label
          className={cn(
            "block text-sm font-semibold text-af-text",
            DASHBOARD_SUPPORTING_LABELS_CLASS,
          )}
          htmlFor="export-factory-image"
        >
          {imageLabel}
        </label>
      }
    />
  );
}
