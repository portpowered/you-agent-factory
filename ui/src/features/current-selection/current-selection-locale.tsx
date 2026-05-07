import type { ReactNode } from "react";
import { createContext, useContext } from "react";

import {
  type CurrentSelectionShellMessages,
  getCurrentSelectionShellMessages,
} from "./messages/current-selection-shell";

const CurrentSelectionLocaleContext =
  createContext<CurrentSelectionShellMessages | null>(null);

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
      value={getCurrentSelectionShellMessages(locale)}
    >
      {children}
    </CurrentSelectionLocaleContext.Provider>
  );
}

export function useCurrentSelectionShellMessages(): CurrentSelectionShellMessages {
  return (
    useContext(CurrentSelectionLocaleContext) ??
    getCurrentSelectionShellMessages(undefined)
  );
}
