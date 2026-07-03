import {
  PackageCheckbox,
  PackageFileInput,
  PackageInput,
  PackageTextarea,
  inputVariants as packageInputVariants,
  textareaVariants as packageTextareaVariants,
} from "@you-agent-factory/components";

import { Checkbox } from "./checkbox";
import { FileInput } from "./file-input";
import { Input, inputVariants } from "./input";
import { Textarea, textareaVariants } from "./textarea";

describe("dashboard input primitive package migration", () => {
  it("re-exports package primitives through dashboard ui entrypoints", () => {
    expect(Input).toBe(PackageInput);
    expect(Textarea).toBe(PackageTextarea);
    expect(Checkbox).toBe(PackageCheckbox);
    expect(FileInput).toBe(PackageFileInput);
    expect(inputVariants).toBe(packageInputVariants);
    expect(textareaVariants).toBe(packageTextareaVariants);
  });
});
