import type { ReactNode } from "react";
import { createContext, useContext } from "react";
import { getWorkstationDetailMessages } from "../../../messages/workstation-detail";
import type { WorkstationDetailMessages } from "../../../messages/workstation-detail-types";
import {
  type CurrentSelectionDetailMessages,
  getCurrentSelectionDetailMessages,
} from "../../messages/shell/current-selection-detail";
import {
  type CurrentSelectionDispatchHistoryMessages,
  getCurrentSelectionDispatchHistoryMessages,
} from "../../messages/shell/current-selection-dispatch-history";
import {
  type CurrentSelectionOperationalEnumMessages,
  getCurrentSelectionOperationalEnumMessages,
} from "../../messages/operational/current-selection-operational-enums";
import {
  type CurrentSelectionShellMessages,
  getCurrentSelectionShellMessages,
} from "../../messages/shell/current-selection-shell";

interface CurrentSelectionLocaleMessages {
  detail: CurrentSelectionDetailMessages;
  dispatchHistory: CurrentSelectionDispatchHistoryMessages;
  operationalEnums: CurrentSelectionOperationalEnumMessages;
  locale?: string | null;
  shell: CurrentSelectionShellMessages;
  workstationDetail: WorkstationDetailMessages;
}

const CurrentSelectionLocaleContext =
  createContext<CurrentSelectionLocaleMessages | null>(null);

export interface CurrentSelectionLocaleProviderProps {
  children: ReactNode;
  locale?: string | null;
}

export function CurrentSelectionLocaleProvider({
  children,
  locale,
}: CurrentSelectionLocaleProviderProps) {
  return (
    <CurrentSelectionLocaleContext.Provider
      value={{
        detail: getCurrentSelectionDetailMessages(locale),
        dispatchHistory: getCurrentSelectionDispatchHistoryMessages(locale),
        operationalEnums: getCurrentSelectionOperationalEnumMessages(locale),
        locale,
        shell: getCurrentSelectionShellMessages(locale),
        workstationDetail: getWorkstationDetailMessages(locale),
      }}
    >
      {children}
    </CurrentSelectionLocaleContext.Provider>
  );
}

export function useCurrentSelectionShellMessages(): CurrentSelectionShellMessages {
  return (
    useContext(CurrentSelectionLocaleContext)?.shell ??
    getCurrentSelectionShellMessages(undefined)
  );
}

export function useCurrentSelectionDispatchHistoryMessages(): CurrentSelectionDispatchHistoryMessages {
  return (
    useContext(CurrentSelectionLocaleContext)?.dispatchHistory ??
    getCurrentSelectionDispatchHistoryMessages(undefined)
  );
}

export function useCurrentSelectionDetailMessages(): CurrentSelectionDetailMessages {
  return (
    useContext(CurrentSelectionLocaleContext)?.detail ??
    getCurrentSelectionDetailMessages(undefined)
  );
}

export function useCurrentSelectionOperationalEnumMessages(): CurrentSelectionOperationalEnumMessages {
  return (
    useContext(CurrentSelectionLocaleContext)?.operationalEnums ??
    getCurrentSelectionOperationalEnumMessages(undefined)
  );
}

export function useCurrentSelectionWorkstationDetailMessages(): WorkstationDetailMessages {
  return (
    useContext(CurrentSelectionLocaleContext)?.workstationDetail ??
    getWorkstationDetailMessages(undefined)
  );
}

export function useCurrentSelectionLocale(): string | null | undefined {
  return useContext(CurrentSelectionLocaleContext)?.locale;
}
