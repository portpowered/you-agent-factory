import {
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";

import {
  type PackagedFactoryDetailViewModel,
  type PackagedFactoryInventoryViewModel,
  projectPackagedFactoryDetail,
  projectPackagedFactoryInventory,
} from "../lib/projection";
import {
  loadPackagedFactoryCatalog,
  type PackagedFactoryCatalogOutcome,
  type PackagedFactoryPublicDataSource,
  resolvePackagedFactorySelection,
  type SelectedArtifactFailure,
} from "../lib/public-contract";

type ReadyCatalog = Extract<
  PackagedFactoryCatalogOutcome,
  { readonly status: "ready" }
>;

type SelectionState =
  | { readonly status: "loading" }
  | {
      readonly status: "ready";
      readonly detail: PackagedFactoryDetailViewModel;
    }
  | {
      readonly status: "error";
      readonly failure: SelectedArtifactFailure;
    };

type InventoryState =
  | { readonly status: "loading" }
  | { readonly status: "empty" }
  | { readonly status: "invalid-contract" }
  | {
      readonly status: "unsupported-version";
      readonly formatVersion: string;
    }
  | {
      readonly status: "ready";
      readonly catalog: ReadyCatalog;
      readonly inventory: PackagedFactoryInventoryViewModel;
      readonly selectedIdentity: string;
      readonly selection: SelectionState;
    };

interface ScopedInventoryState {
  readonly locale: string;
  readonly source: PackagedFactoryPublicDataSource;
  readonly value: InventoryState;
}

type ScopedStateSetter = Dispatch<SetStateAction<ScopedInventoryState>>;

export interface PackagedFactoryInventoryController {
  readonly select: (identity: string) => void;
  readonly state: InventoryState;
}

function catalogOutcomeState(
  outcome: Exclude<PackagedFactoryCatalogOutcome, ReadyCatalog>,
): InventoryState {
  if (outcome.status === "unsupported-version") {
    return {
      status: "unsupported-version",
      formatVersion: outcome.formatVersion,
    };
  }
  return { status: outcome.status };
}

function useSelectionResolver(
  source: PackagedFactoryPublicDataSource,
  locale: string,
  requestID: MutableRefObject<number>,
  setScopedState: ScopedStateSetter,
) {
  return useCallback(
    async (
      catalog: ReadyCatalog,
      inventory: PackagedFactoryInventoryViewModel,
      identity: string,
      activeRequestID: number,
    ) => {
      const item = inventory.byIdentity[identity];
      if (!item) {
        return;
      }
      const outcome = await resolvePackagedFactorySelection(
        source,
        catalog,
        item.slug,
      );
      if (requestID.current !== activeRequestID) {
        return;
      }
      setScopedState({
        locale,
        source,
        value: {
          status: "ready",
          catalog,
          inventory,
          selectedIdentity: identity,
          selection:
            outcome.status === "ready"
              ? {
                  status: "ready",
                  detail: projectPackagedFactoryDetail(outcome, locale),
                }
              : {
                  status: "error",
                  failure: outcome.failure,
                },
        },
      });
    },
    [locale, requestID, setScopedState, source],
  );
}

export function usePackagedFactoryInventory(
  source: PackagedFactoryPublicDataSource,
  locale: string,
): PackagedFactoryInventoryController {
  const requestID = useRef(0);
  const [scopedState, setScopedState] = useState<ScopedInventoryState>({
    locale,
    source,
    value: { status: "loading" },
  });
  const resolveSelection = useSelectionResolver(
    source,
    locale,
    requestID,
    setScopedState,
  );

  useEffect(() => {
    const activeRequestID = ++requestID.current;
    setScopedState({ locale, source, value: { status: "loading" } });

    void loadPackagedFactoryCatalog(source).then((outcome) => {
      if (requestID.current !== activeRequestID) {
        return;
      }
      if (outcome.status !== "ready") {
        setScopedState({
          locale,
          source,
          value: catalogOutcomeState(outcome),
        });
        return;
      }

      const inventory = projectPackagedFactoryInventory(outcome, locale);
      const selectedIdentity = inventory.items[0]?.identity;
      if (!selectedIdentity) {
        setScopedState({ locale, source, value: { status: "empty" } });
        return;
      }

      setScopedState({
        locale,
        source,
        value: {
          status: "ready",
          catalog: outcome,
          inventory,
          selectedIdentity,
          selection: { status: "loading" },
        },
      });
      void resolveSelection(
        outcome,
        inventory,
        selectedIdentity,
        activeRequestID,
      );
    });

    return () => {
      requestID.current += 1;
    };
  }, [locale, resolveSelection, source]);

  const select = useCallback(
    (identity: string) => {
      const state =
        scopedState.source === source && scopedState.locale === locale
          ? scopedState.value
          : { status: "loading" as const };
      if (state.status !== "ready" || !state.inventory.byIdentity[identity]) {
        return;
      }

      const activeRequestID = ++requestID.current;
      setScopedState({
        locale,
        source,
        value: {
          ...state,
          selectedIdentity: identity,
          selection: { status: "loading" },
        },
      });
      void resolveSelection(
        state.catalog,
        state.inventory,
        identity,
        activeRequestID,
      );
    },
    [locale, resolveSelection, scopedState, source],
  );

  return {
    select,
    state:
      scopedState.source === source && scopedState.locale === locale
        ? scopedState.value
        : { status: "loading" },
  };
}
