// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import {
  FormDescription,
  FormError,
  FormField,
  FormFieldGroup,
  FormFieldGroupLabel,
  FormHelperText,
  FormLabel,
  FormSuccess,
  FormWarning,
} from "@you-agent-factory/components";
import {
  FormDescription as FormsFormDescription,
  FormError as FormsFormError,
  FormField as FormsFormField,
  FormFieldGroup as FormsFormFieldGroup,
  FormFieldGroupLabel as FormsFormFieldGroupLabel,
  FormHelperText as FormsFormHelperText,
  FormLabel as FormsFormLabel,
  FormSuccess as FormsFormSuccess,
  FormWarning as FormsFormWarning,
} from "@you-agent-factory/components/forms";

describe("@you-agent-factory/components form-field imports", () => {
  it("imports form-field messaging from the package root", () => {
    expect(FormField).toBeTruthy();
    expect(FormLabel).toBeTruthy();
    expect(FormDescription).toBeTruthy();
    expect(FormHelperText).toBeTruthy();
    expect(FormError).toBeTruthy();
    expect(FormWarning).toBeTruthy();
    expect(FormSuccess).toBeTruthy();
    expect(FormFieldGroup).toBeTruthy();
    expect(FormFieldGroupLabel).toBeTruthy();
  });

  it("imports the same form-field exports from the forms category entrypoint", () => {
    expect(FormsFormField).toBe(FormField);
    expect(FormsFormLabel).toBe(FormLabel);
    expect(FormsFormDescription).toBe(FormDescription);
    expect(FormsFormHelperText).toBe(FormHelperText);
    expect(FormsFormError).toBe(FormError);
    expect(FormsFormWarning).toBe(FormWarning);
    expect(FormsFormSuccess).toBe(FormSuccess);
    expect(FormsFormFieldGroup).toBe(FormFieldGroup);
    expect(FormsFormFieldGroupLabel).toBe(FormFieldGroupLabel);
  });
});
