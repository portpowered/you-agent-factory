import {
  forwardRef,
  type HTMLAttributes,
  type TableHTMLAttributes,
  type TdHTMLAttributes,
  type ThHTMLAttributes,
} from "react";

import { cn } from "../utilities/cn";

export type TableSize = "default" | "dense";

export interface TableProps extends TableHTMLAttributes<HTMLTableElement> {
  containerClassName?: string;
  containerProps?: HTMLAttributes<HTMLDivElement>;
  size?: TableSize;
}

export const Table = forwardRef<HTMLTableElement, TableProps>(function Table(
  { className, containerClassName, containerProps, size = "default", ...props },
  ref,
) {
  return (
    <div
      className={cn(
        "group/table w-full overflow-x-auto overflow-y-clip rounded-2xl border border-outline",
        containerClassName,
      )}
      data-size={size}
      {...containerProps}
    >
      <table
        className={cn("w-full caption-bottom text-sm", className)}
        ref={ref}
        {...props}
      />
    </div>
  );
});

export const TableHeader = forwardRef<
  HTMLTableSectionElement,
  HTMLAttributes<HTMLTableSectionElement>
>(function TableHeader({ className, ...props }, ref) {
  return (
    <thead
      className={cn("[&_tr]:border-b [&_tr]:border-outline", className)}
      ref={ref}
      {...props}
    />
  );
});

export const TableBody = forwardRef<
  HTMLTableSectionElement,
  HTMLAttributes<HTMLTableSectionElement>
>(function TableBody({ className, ...props }, ref) {
  return (
    <tbody
      className={cn(
        "[&_tr:last-child]:border-0 [&_tr]:border-b [&_tr]:border-outline",
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});

export const TableRow = forwardRef<
  HTMLTableRowElement,
  HTMLAttributes<HTMLTableRowElement>
>(function TableRow({ className, ...props }, ref) {
  return (
    <tr
      className={cn(
        "transition-colors hover:bg-af-overlay data-[state=selected]:bg-af-overlay",
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});

export const TableHead = forwardRef<
  HTMLTableCellElement,
  ThHTMLAttributes<HTMLTableCellElement>
>(function TableHead({ className, ...props }, ref) {
  return (
    <th
      className={cn(
        "h-11 px-4 text-left align-middle text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle",
        "group-data-[size=dense]/table:h-8 group-data-[size=dense]/table:px-3 group-data-[size=dense]/table:py-2",
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});

export const TableCell = forwardRef<
  HTMLTableCellElement,
  TdHTMLAttributes<HTMLTableCellElement>
>(function TableCell({ className, ...props }, ref) {
  return (
    <td
      className={cn(
        "min-w-0 px-4 py-3 align-middle text-on-surface",
        "group-data-[size=dense]/table:px-3 group-data-[size=dense]/table:py-2",
        className,
      )}
      ref={ref}
      {...props}
    />
  );
});

export const TableCaption = forwardRef<
  HTMLTableCaptionElement,
  HTMLAttributes<HTMLTableCaptionElement>
>(function TableCaption({ className, ...props }, ref) {
  return (
    <caption
      className={cn("mt-4 text-sm text-af-text-subtle", className)}
      ref={ref}
      {...props}
    />
  );
});
