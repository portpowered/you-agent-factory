import {
  type FactoryGraphVisualNestedAccentRole,
  type FactoryGraphVisualStatusRole,
  factoryGraphVisualNestedAccentRole,
} from "@you-agent-factory/factory-graph";
import { describe, expect, it } from "vitest";

const PARENT_STATUS_BACKING_ROLES: ReadonlyArray<
  readonly [
    parentStatus: FactoryGraphVisualStatusRole,
    backingRole: FactoryGraphVisualNestedAccentRole,
  ]
> = [
  ["quiet", "neutral"],
  ["waiting", "info"],
  ["active", "warning"],
  ["success", "success"],
  ["danger", "danger"],
];

describe("factory graph nested accent roles", () => {
  it.each(PARENT_STATUS_BACKING_ROLES)(
    "maps the %s parent status to its %s semantic backing role",
    (parentStatus, backingRole) => {
      expect(factoryGraphVisualNestedAccentRole(parentStatus)).toBe(
        backingRole,
      );
    },
  );
});
