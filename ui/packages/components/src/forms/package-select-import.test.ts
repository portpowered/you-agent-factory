// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import {
  EnumSelect,
  NativeSelect,
  OptionalEnumSelect,
  ResetEnumSelect,
  Select,
  SelectContent,
  SelectField,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@you-agent-factory/components";
import {
  EnumSelect as FormsEnumSelect,
  NativeSelect as FormsNativeSelect,
  OptionalEnumSelect as FormsOptionalEnumSelect,
  ResetEnumSelect as FormsResetEnumSelect,
  Select as FormsSelect,
  SelectContent as FormsSelectContent,
  SelectField as FormsSelectField,
  SelectGroup as FormsSelectGroup,
  SelectItem as FormsSelectItem,
  SelectLabel as FormsSelectLabel,
  SelectSeparator as FormsSelectSeparator,
  SelectTrigger as FormsSelectTrigger,
  SelectValue as FormsSelectValue,
} from "@you-agent-factory/components/forms";

describe("@you-agent-factory/components select imports", () => {
  it("imports select primitives from the package root", () => {
    expect(Select).toBeTruthy();
    expect(SelectGroup).toBeTruthy();
    expect(SelectValue).toBeTruthy();
    expect(typeof SelectTrigger).toBe("object");
    expect(SelectContent).toBeTypeOf("function");
    expect(SelectLabel).toBeTypeOf("function");
    expect(typeof SelectItem).toBe("object");
    expect(SelectSeparator).toBeTypeOf("function");
    expect(SelectField).toBeTypeOf("function");
    expect(typeof NativeSelect).toBe("object");
    expect(EnumSelect).toBeTypeOf("function");
    expect(OptionalEnumSelect).toBeTypeOf("function");
    expect(ResetEnumSelect).toBeTypeOf("function");
  });

  it("imports the same select primitives from the forms category entrypoint", () => {
    expect(FormsSelect).toBe(Select);
    expect(FormsSelectGroup).toBe(SelectGroup);
    expect(FormsSelectValue).toBe(SelectValue);
    expect(FormsSelectTrigger).toBe(SelectTrigger);
    expect(FormsSelectContent).toBe(SelectContent);
    expect(FormsSelectLabel).toBe(SelectLabel);
    expect(FormsSelectItem).toBe(SelectItem);
    expect(FormsSelectSeparator).toBe(SelectSeparator);
    expect(FormsSelectField).toBe(SelectField);
    expect(FormsNativeSelect).toBe(NativeSelect);
    expect(FormsEnumSelect).toBe(EnumSelect);
    expect(FormsOptionalEnumSelect).toBe(OptionalEnumSelect);
    expect(FormsResetEnumSelect).toBe(ResetEnumSelect);
  });
});
