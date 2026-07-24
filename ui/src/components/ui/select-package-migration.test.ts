import {
  EnumSelect,
  NativeSelect,
  OptionalEnumSelect,
  ResetEnumSelect,
  Select,
  SelectContent,
  SelectField,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@you-agent-factory/components";
import { describe, expect, it } from "vitest";
import {
  EnumSelect as DashboardEnumSelect,
  OptionalEnumSelect as DashboardOptionalEnumSelect,
  ResetEnumSelect as DashboardResetEnumSelect,
} from "./enum-select";
import { NativeSelect as DashboardNativeSelect } from "./native-select";
import {
  Select as DashboardSelect,
  SelectContent as DashboardSelectContent,
  SelectField as DashboardSelectField,
  SelectItem as DashboardSelectItem,
  SelectTrigger as DashboardSelectTrigger,
  SelectValue as DashboardSelectValue,
} from "./select";

describe("dashboard select package migration", () => {
  it("re-exports select primitives from @you-agent-factory/components", () => {
    expect(DashboardSelect).toBe(Select);
    expect(DashboardSelectContent).toBe(SelectContent);
    expect(DashboardSelectField).toBe(SelectField);
    expect(DashboardSelectItem).toBe(SelectItem);
    expect(DashboardSelectTrigger).toBe(SelectTrigger);
    expect(DashboardSelectValue).toBe(SelectValue);
  });

  it("re-exports enum and native select helpers from @you-agent-factory/components", () => {
    expect(DashboardEnumSelect).toBe(EnumSelect);
    expect(DashboardOptionalEnumSelect).toBe(OptionalEnumSelect);
    expect(DashboardResetEnumSelect).toBe(ResetEnumSelect);
    expect(DashboardNativeSelect).toBe(NativeSelect);
  });
});
