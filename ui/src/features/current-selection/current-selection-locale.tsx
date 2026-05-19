import type { ReactNode } from "react";
import { createContext, useContext } from "react";

import {
  type CurrentSelectionDispatchHistoryMessages,
  getCurrentSelectionDispatchHistoryMessages,
} from "./messages/current-selection-dispatch-history";
import {
  type CurrentSelectionDetailMessages,
  getCurrentSelectionDetailMessages,
} from "./messages/current-selection-detail";
import {
  type CurrentSelectionShellMessages,
  getCurrentSelectionShellMessages,
} from "./messages/current-selection-shell";

interface CurrentSelectionLocaleMessages {
  detail: CurrentSelectionDetailMessages;
  dispatchHistory: CurrentSelectionDispatchHistoryMessages;
  shell: CurrentSelectionShellMessages;
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
        shell: getCurrentSelectionShellMessages(locale),
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
