import {
  Children,
  cloneElement,
  isValidElement,
  type ReactNode,
  type ReactElement,
  type ComponentProps,
} from "react";
import "./responsive-table.css";
type Element = ReactElement<{
  children?: ReactNode;
  colSpan?: number;
  "data-label"?: string;
}>;
const text = (node: ReactNode): string =>
  Children.toArray(node)
    .map((child) =>
      isValidElement(child)
        ? text((child as Element).props.children)
        : typeof child === "string" || typeof child === "number"
          ? String(child)
          : "",
    )
    .join("")
    .trim();
// Preserve table semantics and reuse translated headings as mobile field labels.
export function ResponsiveTable({
  children,
  className = "",
  ...props
}: ComponentProps<"table">) {
  const sections = Children.toArray(children);
  const head = sections.find(
    (child) => isValidElement(child) && child.type === "thead",
  ) as Element | undefined;
  const headerRows = Children.toArray(head?.props.children);
  const headers = headerRows.flatMap((row) =>
    isValidElement(row)
      ? Children.toArray((row as Element).props.children).map(text)
      : [],
  );
  return (
    <table {...props} className={`responsive-table ${className}`} role="table">
      {sections.map((section) => {
        if (!isValidElement(section) || section.type !== "tbody")
          return section;
        return cloneElement(
          section as Element,
          {},
          Children.map((section as Element).props.children, (row) => {
            if (!isValidElement(row) || row.type !== "tr") return row;
            return cloneElement(
              row as Element,
              {},
              Children.map((row as Element).props.children, (cell, index) => {
                if (
                  !isValidElement(cell) ||
                  cell.type !== "td" ||
                  (cell as Element).props.colSpan
                )
                  return cell;
                return cloneElement(cell as Element, {
                  "data-label": headers[index],
                });
              }),
            );
          }),
        );
      })}
    </table>
  );
}
