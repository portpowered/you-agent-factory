import { Text } from "@you-agent-factory/components/primitives";
import { FileInput } from "../../../components/ui/file-input";
import {
  FormDescription,
  FormError,
  FormLabel,
} from "../../../components/ui/form-field";
import { ChooseFileField } from "../../choose-file/components/choose-file-field";

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
            <Text className="m-0" variant="supporting">
              {selectedImageLabel(selectedImage.name)}
            </Text>
          ) : null}
          {imageValidationMessage ? (
            <FormError id={imageValidationId}>
              {imageValidationMessage}
            </FormError>
          ) : null}
        </>
      }
      control={
        <FileInput
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
      description={<FormDescription>{imageDescription}</FormDescription>}
      disabled={isExporting}
      label={<FormLabel htmlFor="export-factory-image">{imageLabel}</FormLabel>}
    />
  );
}
