import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";

import {
  getPackagedFactoryCatalog,
  type PackagedFactoryCatalogResponse,
} from "../../../api/packaged-factories";
import {
  type PackagedFactoryDetailViewModel,
  type PackagedFactoryInventoryViewModel,
  projectPackagedFactoryDetail,
  projectPackagedFactoryInventory,
} from "../lib/projection";

type SelectionState =
  | { readonly status: "error" }
  | {
      readonly status: "ready";
      readonly detail: PackagedFactoryDetailViewModel;
    };

type InventoryState =
  | { readonly status: "loading" }
  | { readonly status: "empty" }
  | { readonly status: "error" }
  | {
      readonly status: "ready";
      readonly inventory: PackagedFactoryInventoryViewModel;
      readonly selectedIdentity: string;
      readonly selection: SelectionState;
    };

export interface PackagedFactoryInventoryController {
  readonly retry: () => void;
  readonly select: (identity: string) => void;
  readonly state: InventoryState;
}

const packagedFactoryCatalogQueryKey = [
  "packaged-factories",
  "catalog",
] as const;

function projectCatalogState(
  catalog: PackagedFactoryCatalogResponse,
  locale: string,
  selectedIdentity: string | undefined,
): InventoryState {
  const inventory = projectPackagedFactoryInventory(catalog, locale);
  const firstIdentity = inventory.items[0]?.identity;
  if (!firstIdentity) {
    return { status: "empty" };
  }

  const effectiveIdentity =
    selectedIdentity && inventory.byIdentity[selectedIdentity]
      ? selectedIdentity
      : firstIdentity;
  const selectedEntry = catalog.factories.find(
    (entry) => entry.name === effectiveIdentity,
  );
  const detail = selectedEntry
    ? projectPackagedFactoryDetail(selectedEntry, locale)
    : undefined;

  return {
    status: "ready",
    inventory,
    selectedIdentity: effectiveIdentity,
    selection: detail ? { status: "ready", detail } : { status: "error" },
  };
}

export function usePackagedFactoryInventory(
  locale: string,
): PackagedFactoryInventoryController {
  const [selectedIdentity, setSelectedIdentity] = useState<string>();
  const catalogQuery = useQuery({
    queryKey: packagedFactoryCatalogQueryKey,
    queryFn: ({ signal }) => getPackagedFactoryCatalog({ signal }),
    retry: false,
    staleTime: Infinity,
  });

  const inventory = useMemo(() => {
    if (!catalogQuery.data) {
      return undefined;
    }
    return projectPackagedFactoryInventory(catalogQuery.data, locale);
  }, [catalogQuery.data, locale]);

  useEffect(() => {
    if (!inventory) {
      setSelectedIdentity(undefined);
      return;
    }

    setSelectedIdentity((current) =>
      current && inventory.byIdentity[current]
        ? current
        : inventory.items[0]?.identity,
    );
  }, [inventory]);

  const select = useCallback(
    (identity: string) => {
      if (inventory?.byIdentity[identity]) {
        setSelectedIdentity(identity);
      }
    },
    [inventory],
  );
  const retry = useCallback(() => {
    void catalogQuery.refetch();
  }, [catalogQuery.refetch]);

  let state: InventoryState;
  if (catalogQuery.isPending) {
    state = { status: "loading" };
  } else if (catalogQuery.isError || !catalogQuery.data) {
    state = { status: "error" };
  } else {
    state = projectCatalogState(catalogQuery.data, locale, selectedIdentity);
  }

  return { retry, select, state };
}
