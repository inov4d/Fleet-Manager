import type { HTMLAttributes, TdHTMLAttributes, ThHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export function Table({ className, children, ...rest }: HTMLAttributes<HTMLTableElement>) {
  return (
    <div className="w-full overflow-x-auto">
      <table className={cn("w-full text-xs border-collapse", className)} {...rest}>
        {children}
      </table>
    </div>
  );
}

export function THead({ children, className, ...rest }: HTMLAttributes<HTMLTableSectionElement>) {
  return (
    <thead className={cn("bg-surface-2 text-fg-muted sticky top-0 z-[1]", className)} {...rest}>
      {children}
    </thead>
  );
}

export function TBody({ children, ...rest }: HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody {...rest}>{children}</tbody>;
}

export function Tr({ className, children, ...rest }: HTMLAttributes<HTMLTableRowElement>) {
  return (
    <tr className={cn("border-b border-border hover:bg-surface-2/60", className)} {...rest}>
      {children}
    </tr>
  );
}

export function Th({ className, children, ...rest }: ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th scope="col" className={cn("text-left font-medium px-2 py-1.5 whitespace-nowrap", className)} {...rest}>
      {children}
    </th>
  );
}

export function Td({ className, children, ...rest }: TdHTMLAttributes<HTMLTableCellElement>) {
  return (
    <td className={cn("px-2 py-1.5 align-middle", className)} {...rest}>
      {children}
    </td>
  );
}
