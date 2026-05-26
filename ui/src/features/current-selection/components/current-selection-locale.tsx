import type { ReactNode } from "react";
import { createContext, useContext } from "react";

import {
  type CurrentSelectionDispatchHistoryMessages,
  getCurrentSelectionDispatchHistoryMessages,
} from "../messages/current-selection-dispatch-history";
import {
  type CurrentSelectionDetailMessages,
  getCurrentSelectionDetailMessages,
} from "../messages/current-selection-detail";
import {
  type CurrentSelectionShellMessages,
  getCurrentSelectionShellMessages,
} from "../messages/current-selection-shell";
import {
  type CurrentSelectionOperationalEnumMessages,
  getCurrentSelectionOperationalEnumMessages,
} from "../messages/current-selection-operational-enums";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";

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
