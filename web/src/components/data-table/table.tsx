import type { CSSProperties, ReactNode } from "react";
import type { Columns } from "./columns";
import { cn } from "@/lib/utils";

const RowWrapper = ({
  children,
  className = "",
  onClick,
  style,
}: {
  children: ReactNode;
  className?: string;
  onClick?: () => void;
  style?: CSSProperties;
}) => (
  <div className={cn("grid w-full", className)} onClick={onClick} style={style}>
    {children}
  </div>
);

const ColumnRowItem = ({
  children,
  className = "",
  style,
}: {
  children: ReactNode;
  className?: string;
  style?: CSSProperties;
}) => (
  <div
    className={cn(
      "flex-shrink-0 overflow-hidden border-b border-neutral-200 px-3 py-3",
      className
    )}
    style={style}
  >
    {children}
  </div>
);

function RenderLoader({
  columns,
  style,
}: {
  columns: string[];
  style?: CSSProperties;
}) {
  return Array.from({ length: 5 }).map((_, index) => (
    <RowWrapper
      key={`loader-row-${index}`}
      className="animate-pulse"
      style={style}
    >
      {columns.map((_, cIndex) => (
        <ColumnRowItem
          key={`loader-column-${cIndex}`}
          className="py-4 group-last-of-type:border-b-0"
        >
          <div className="h-2 w-2/3 rounded bg-neutral-800/15" />
        </ColumnRowItem>
      ))}
    </RowWrapper>
  ));
}

/** CSS-grid data table (sticky header, hover rows). */
export default function DataTable({
  columns,
  data,
  isFetched = true,
  isLoading = false,
  visibilityColumns,
  onClick,
  extraFunctions = {},
  emptyTitle = "No data",
  emptyBody,
}: {
  columns: Columns<string, any>;
  data: any[];
  isFetched?: boolean;
  isLoading?: boolean;
  visibilityColumns: Record<string, boolean>;
  onClick?: (item: any) => void;
  extraFunctions?: Record<string, unknown>;
  emptyTitle?: string;
  emptyBody?: string;
}) {
  const visibleColumns = Object.entries(visibilityColumns)
    .filter(([key, value]) => value && columns[key])
    .map(([key]) => key);
  const noData = !data?.length && !isLoading && isFetched;

  const style: CSSProperties = {
    gridTemplateColumns: visibleColumns
      .map((column) => columns[column]?.width || "minmax(0, 1fr)")
      .join(" "),
  };

  return (
    <div className="flex w-full grow flex-col overflow-auto rounded-md border border-neutral-200 bg-neutral-50/50">
      <RowWrapper className="sticky top-0 z-10" style={style}>
        {visibleColumns.map((column) => (
          <ColumnRowItem
            key={column}
            className="whitespace-nowrap bg-neutral-100 text-sm text-neutral-500 group-last-of-type:border-b-0"
          >
            {columns[column]?.header()}
          </ColumnRowItem>
        ))}
      </RowWrapper>
      <div
        className={cn(
          "flex w-full flex-col",
          isFetched && isLoading ? "animate-pulse" : ""
        )}
      >
        {(!isFetched || (isLoading && !data?.length)) && (
          <RenderLoader columns={visibleColumns} style={style} />
        )}
        {data?.map((rowItem, index) => (
          <RowWrapper
            key={`row-${index}`}
            className={cn(
              "group text-sm text-neutral-700",
              onClick ? "cursor-pointer" : ""
            )}
            onClick={onClick ? () => onClick(rowItem) : undefined}
            style={style}
          >
            {visibleColumns.map((column, cIdx) => (
              <ColumnRowItem
                key={`row-${index}-column-${cIdx}`}
                className="group-hover:bg-neutral-100 group-hover:text-neutral-800"
              >
                {columns[column]?.cell({
                  row: rowItem,
                  extraFunctions,
                })}
              </ColumnRowItem>
            ))}
          </RowWrapper>
        ))}
        {noData && (
          <div className="px-6 py-10 text-center">
            <p className="text-sm font-medium text-neutral-800">{emptyTitle}</p>
            {emptyBody && (
              <p className="mx-auto mt-1.5 max-w-md text-xs text-neutral-500">
                {emptyBody}
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
