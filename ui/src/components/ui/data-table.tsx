import type { ReactNode } from "react";

import {
  DataTable as PackageDataTable,
  type DataTableColumn,
  type DataTableProps as PackageDataTableProps,
  type DataTableState,
} from "@you-agent-factory/components/data-display";
import { useAppLocale } from "../../i18n";
import { getSharedPrimitiveMessages } from "./messages/shared-primitives";

export type { DataTableColumn, DataTableState };

export interface DataTableProps<Row>
  extends Omit<PackageDataTableProps<Row>, "emptyMessage"> {
  emptyMessage?: ReactNode;
  locale?: string | null;
}

export function DataTable<Row>({
  emptyMessage,
  locale: localeOverride,
  ...props
}: DataTableProps<Row>) {
  const { locale } = useAppLocale(localeOverride);
  const messages = getSharedPrimitiveMessages(locale);

  return (
    <PackageDataTable
      emptyMessage={emptyMessage ?? messages.emptyTableMessage}
      {...props}
    />
  );
}
