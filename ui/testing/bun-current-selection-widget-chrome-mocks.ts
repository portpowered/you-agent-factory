/**
 * Widget chrome mocks for current-selection specs that stub save flow without
 * exercising the real save hook. Do not import from save-flow specs.
 */
import { mock } from "bun:test";

import "./bun-current-factory-definition-public-mocks";
import {
  useCurrentWorkstationPromptTemplateContractMock,
  useCurrentWorkstationPromptTemplateValidationMock,
} from "./bun-current-selection-isolated-hook-mocks";

export {
  useCurrentWorkstationPromptTemplateContractMock,
  useCurrentWorkstationPromptTemplateValidationMock,
};

const SAVE_EDITABLE_WORKSTATION_CONFIGURATION_HOOK_MODULE =
  "../src/features/current-selection/workstation-selection/hooks/use-save-editable-workstation-configuration";

const saveEditableWorkstationConfigurationHookActual = await import(
  SAVE_EDITABLE_WORKSTATION_CONFIGURATION_HOOK_MODULE,
);

export const useSaveEditableWorkstationConfigurationMock = mock(() => ({
  beginSaveConfirmation: () => undefined,
  canSave: false,
  cancelSaveConfirmation: () => undefined,
  confirmSave: () => undefined,
  saveState: { status: "idle" },
}));

mock.module(SAVE_EDITABLE_WORKSTATION_CONFIGURATION_HOOK_MODULE, () => ({
  ...saveEditableWorkstationConfigurationHookActual,
  useSaveEditableWorkstationConfiguration:
    useSaveEditableWorkstationConfigurationMock,
}));
