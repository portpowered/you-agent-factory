import {
  DataTable as PackageDataTable,
  type DataTableProps as PackageDataTableProps,
} from "@you-agent-factory/components/data-display";
import type { ReactNode } from "react";
import { useAppLocale } from "../../i18n";
import { getSharedPrimitiveMessages } from "./messages/shared-primitives";

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
