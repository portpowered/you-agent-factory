import type { ReactNode } from "react";
import { createContext, useContext } from "react";

import {
  type CurrentSelectionDispatchHistoryMessages,
  getCurrentSelectionDispatchHistoryMessages,
} from "./messages/current-selection-dispatch-history";
import {
  type CurrentSelectionShellMessages,
  getCurrentSelectionShellMessages,
} from "./messages/current-selection-shell";
import {
  type WorkstationDetailMessages,
  getWorkstationDetailMessages,
} from "./messages";

interface CurrentSelectionLocaleMessages {
  dispatchHistory: CurrentSelectionDispatchHistoryMessages;
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
        dispatchHistory: getCurrentSelectionDispatchHistoryMessages(locale),
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

export function useCurrentSelectionWorkstationDetailMessages(): WorkstationDetailMessages {
  return (
    useContext(CurrentSelectionLocaleContext)?.workstationDetail ??
    getWorkstationDetailMessages(undefined)
  );
}
