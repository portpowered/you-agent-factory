import { DashboardLabel, DashboardText } from "../../../components/ui";
import {
  ChooseFileField,
  ChooseFileNativeInput,
} from "../../choose-file/public";

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
            <DashboardText className="m-0" variant="supporting">
              {selectedImageLabel(selectedImage.name)}
            </DashboardText>
          ) : null}
          {imageValidationMessage ? (
            <DashboardText
              className="m-0 text-sm font-medium text-on-error-container"
              id={imageValidationId}
              variant="supporting"
            >
              {imageValidationMessage}
            </DashboardText>
          ) : null}
        </>
      }
      control={
        <ChooseFileNativeInput
          accept="image/*"
          aria-describedby={
            imageValidationMessage ? imageValidationId : undefined
          }
          aria-invalid={imageValidationMessage ? "true" : undefined}
          disabled={isExporting}
          id="export-factory-image"
          onChange={(event) => {
            if (isExporting) {
              return;
            }
            onInteraction();
            onImageChange(event.target.files);
          }}
        />
      }
      description={
        <DashboardText className="m-0" variant="supporting">
          {imageDescription}
        </DashboardText>
      }
      disabled={isExporting}
      label={
        <DashboardLabel
          as="label"
          className="block text-sm font-semibold text-on-surface"
          htmlFor="export-factory-image"
        >
          {imageLabel}
        </DashboardLabel>
      }
    />
  );
}
