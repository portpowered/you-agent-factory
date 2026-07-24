// @vitest-environment happy-dom

import {
  inputVariants,
  PackageCheckbox,
  PackageFileInput,
  PackageInput,
  PackageTextarea,
  textareaVariants,
} from "@you-agent-factory/components";
import {
  PackageCheckbox as FormsPackageCheckbox,
  PackageFileInput as FormsPackageFileInput,
  PackageInput as FormsPackageInput,
  PackageTextarea as FormsPackageTextarea,
  inputVariants as formsInputVariants,
  textareaVariants as formsTextareaVariants,
} from "@you-agent-factory/components/forms";
import { describe, expect, it } from "vitest";

describe("@you-agent-factory/components input primitive imports", () => {
  it("imports input primitives from the package root", () => {
    expect(PackageInput).toBeTruthy();
    expect(PackageTextarea).toBeTruthy();
    expect(PackageCheckbox).toBeTruthy();
    expect(PackageFileInput).toBeTruthy();
    expect(typeof PackageInput).toBe("object");
    expect(inputVariants()).toContain("border-outline");
    expect(textareaVariants()).toContain("min-h-28");
  });

  it("imports the same primitives from the forms category entrypoint", () => {
    expect(FormsPackageInput).toBe(PackageInput);
    expect(FormsPackageTextarea).toBe(PackageTextarea);
    expect(FormsPackageCheckbox).toBe(PackageCheckbox);
    expect(FormsPackageFileInput).toBe(PackageFileInput);
    expect(formsInputVariants).toBe(inputVariants);
    expect(formsTextareaVariants).toBe(textareaVariants);
  });
});
