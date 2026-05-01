import { forwardRef, type HTMLAttributes, type TableHTMLAttributes, type TdHTMLAttributes, type ThHTMLAttributes } from "react";

import { cn } from "../../lib/cn";

export const Table = forwardRef<HTMLTableElement, TableHTMLAttributes<HTMLTableElement>>(function Table(
  { className, ...props },
  ref,
) {
  return (
    <div className="w-full overflow-x-auto rounded-2xl border border-af-overlay/10 bg-af-surface/56">
      <table className={cn("w-full caption-bottom text-sm", className)} ref={ref} {...props} />
    </div>
  );
});

export const TableHeader = forwardRef<HTMLTableSectionElement, HTMLAttributes<HTMLTableSectionElement>>(
  function TableHeader({ className, ...props }, ref) {
    return <thead className={cn("[&_tr]:border-b [&_tr]:border-af-overlay/10", className)} ref={ref} {...props} />;
  },
);

export const TableBody = forwardRef<HTMLTableSectionElement, HTMLAttributes<HTMLTableSectionElement>>(
  function TableBody({ className, ...props }, ref) {
    return (
      <tbody
        className={cn("[&_tr:last-child]:border-0 [&_tr]:border-b [&_tr]:border-af-overlay/8", className)}
        ref={ref}
        {...props}
      />
    );
  },
);

export const TableRow = forwardRef<HTMLTableRowElement, HTMLAttributes<HTMLTableRowElement>>(function TableRow(
  { className, ...props },
  ref,
) {
  return (
    <tr
      className={cn("transition-colors hover:bg-af-overlay/6 data-[state=selected]:bg-af-accent/10", className)}
      ref={ref}
      {...props}
    />
  );
});

export const TableHead = forwardRef<HTMLTableCellElement, ThHTMLAttributes<HTMLTableCellElement>>(
  function TableHead({ className, ...props }, ref) {
    return (
      <th
        className={cn(
          "h-11 px-4 text-left align-middle text-xs font-bold uppercase tracking-[0.08em] text-af-ink/58",
          className,
        )}
        ref={ref}
        {...props}
      />
    );
  },
);

export const TableCell = forwardRef<HTMLTableCellElement, TdHTMLAttributes<HTMLTableCellElement>>(
  function TableCell({ className, ...props }, ref) {
    return <td className={cn("px-4 py-3 align-middle text-af-ink/82", className)} ref={ref} {...props} />;
  },
);

export const TableCaption = forwardRef<HTMLTableCaptionElement, HTMLAttributes<HTMLTableCaptionElement>>(
  function TableCaption({ className, ...props }, ref) {
    return <caption className={cn("mt-4 text-sm text-af-ink/58", className)} ref={ref} {...props} />;
  },
);
