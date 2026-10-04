import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import {
  type ColumnDef,
  type SortingState,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  getPaginationRowModel,
  useReactTable,
} from "@tanstack/react-table";
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Search,
  ArrowRight,
  GitBranch,
} from "lucide-react";
import { Button } from "../components/ui/button";
import { Input, Select, Status, Tooltip } from "../components/ui/primitives";
import { TierBadge } from "../components/tier-badge";
import { DataStateView } from "../components/ui/states";
import { getRequests, type RequestRecord } from "../mocks/service";
import { usePreferences } from "../lib/preferences";
import { formatMoney } from "../lib/money";
import { placeholders } from "../config/placeholders";

export function Requests({
  onSelect,
  resetKey,
}: {
  onSelect: (record: RequestRecord) => void;
  resetKey: number;
}) {
  return <RequestTable key={resetKey} onSelect={onSelect} />;
}
function RequestTable({
  onSelect,
}: {
  onSelect: (record: RequestRecord) => void;
}) {
  const { t } = useTranslation();
  const { dataState, setDataState, language } = usePreferences();
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [sorting, setSorting] = useState<SortingState>([]);
  const query = useQuery({
    queryKey: ["requests"],
    queryFn: ({ signal }) => getRequests(signal),
  });
  const data = useMemo(
    () =>
      (query.data ?? []).filter(
        (item) =>
          (status === "all" || status === item.status) &&
          (!search ||
            item.trace_id === search.trim() ||
            item.model.toLowerCase().includes(search.trim().toLowerCase())),
      ),
    [query.data, search, status],
  );
  const columns = useMemo<ColumnDef<RequestRecord>[]>(
    () => [
      {
        accessorKey: "model",
        header: t("model"),
        cell: ({ row }) => (
          <div className="model-cell">
            <span className="provider-monogram" aria-hidden="true">
              {row.original.vendor.slice(0, 1)}
            </span>
            <div>
              <b className="ltr">{row.original.model}</b>
              <small>{row.original.vendor}</small>
            </div>
            {row.original.fallback && (
              <Tooltip text={t("fallback")}>
                <span className="fallback-icon" tabIndex={0}>
                  <GitBranch size={14} aria-label={t("fallback")} />
                </span>
              </Tooltip>
            )}
          </div>
        ),
      },
      {
        accessorKey: "tier",
        header: t("tier"),
        cell: ({ row }) => <TierBadge id={row.original.tier} />,
      },
      {
        accessorKey: "status",
        header: t("status"),
        cell: ({ row }) => (
          <Status
            label={t(row.original.status)}
            tone={
              row.original.status === "success"
                ? "success"
                : row.original.status === "failed"
                  ? "danger"
                  : "warning"
            }
          />
        ),
      },
      {
        accessorKey: "duration_ms",
        header: t("duration"),
        cell: ({ row }) => (
          <span className="numeric ltr">
            {(row.original.duration_ms / 1000).toFixed(2)} s
          </span>
        ),
      },
      {
        accessorKey: "cost_nano",
        header: t("cost"),
        sortingFn: (a, b) =>
          BigInt(a.original.cost_nano) === BigInt(b.original.cost_nano)
            ? 0
            : BigInt(a.original.cost_nano) > BigInt(b.original.cost_nano)
              ? 1
              : -1,
        cell: ({ row }) => (
          <span className="numeric ltr">
            {formatMoney(row.original.cost_nano, "call")}
          </span>
        ),
      },
      {
        accessorKey: "time",
        header: t("time"),
        cell: ({ row }) => (
          <span className="numeric ltr">
            {new Intl.DateTimeFormat(
              language === "ar" ? "ar-AE-u-nu-latn" : "en-GB",
              {
                hour: "2-digit",
                minute: "2-digit",
                second: "2-digit",
                hour12: false,
                timeZone: placeholders.region.timeZone,
              },
            ).format(new Date(row.original.time))}
          </span>
        ),
      },
      {
        id: "actions",
        header: "",
        cell: ({ row }) => (
          <Button
            size="icon"
            variant="ghost"
            aria-label={`${t("details")} ${row.original.trace_id}`}
            onClick={() => onSelect(row.original)}
          >
            <ArrowRight className="directional" />
          </Button>
        ),
      },
    ],
    [t, language, onSelect],
  );
  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: { pagination: { pageSize: 6 } },
  });
  const state = query.isPending
    ? "loading"
    : query.isError
      ? "error"
      : dataState === "ready" && data.length === 0
        ? "empty"
        : dataState;
  return (
    <div className="request-panel">
      <div className="panel-header">
        <div>
          <h3>{t("requestTable")}</h3>
          <p>{t("requestTableNote")}</p>
        </div>
        <span className="subtle-count numeric">
          {t("resultsCount", {
            count: dataState === "empty" ? 0 : data.length,
          })}
        </span>
      </div>
      <div className="table-toolbar">
        <div className="search-input">
          <Search size={16} aria-hidden="true" />
          <Input
            aria-label={t("searchRequests")}
            placeholder={t("searchRequests")}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              table.setPageIndex(0);
            }}
          />
        </div>
        <Select
          label={t("filterStatus")}
          value={status}
          onChange={(value) => {
            setStatus(value);
            table.setPageIndex(0);
          }}
          options={[
            { value: "all", label: t("allStatuses") },
            ...["success", "failed", "interrupted"].map((value) => ({
              value,
              label: t(value),
            })),
          ]}
        />
      </div>
      {state !== "ready" ? (
        <DataStateView
          state={state}
          filtered={!!search || status !== "all"}
          onReset={() => {
            setSearch("");
            setStatus("all");
            setDataState("ready");
            void query.refetch();
          }}
        />
      ) : (
        <>
          <div className="table-scroll">
            <table className="data-table">
              <caption className="sr-only">{t("requestTable")}</caption>
              <thead>
                {table.getHeaderGroups().map((group) => (
                  <tr key={group.id}>
                    {group.headers.map((header) => (
                      <th
                        key={header.id}
                        data-column={header.column.id}
                        aria-sort={
                          header.column.getIsSorted() === "asc"
                            ? "ascending"
                            : header.column.getIsSorted() === "desc"
                              ? "descending"
                              : undefined
                        }
                      >
                        {header.column.getCanSort() ? (
                          <button
                            onClick={header.column.getToggleSortingHandler()}
                            aria-label={t("sort", {
                              column: header.column.columnDef.header,
                            })}
                          >
                            {flexRender(
                              header.column.columnDef.header,
                              header.getContext(),
                            )}
                            {header.column.getIsSorted() === "asc" ? (
                              <ArrowUp />
                            ) : header.column.getIsSorted() === "desc" ? (
                              <ArrowDown />
                            ) : (
                              <ArrowUpDown className="sort-idle" />
                            )}
                          </button>
                        ) : (
                          <span className="sr-only">{t("details")}</span>
                        )}
                      </th>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody>
                {table.getRowModel().rows.map((row) => (
                  <tr key={row.id}>
                    {row.getVisibleCells().map((cell) => (
                      <td
                        key={cell.id}
                        data-column={cell.column.id}
                        data-label={
                          typeof cell.column.columnDef.header === "string"
                            ? cell.column.columnDef.header
                            : ""
                        }
                      >
                        {flexRender(
                          cell.column.columnDef.cell,
                          cell.getContext(),
                        )}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="pagination">
            <span>
              {t("pageCount", {
                page: table.getState().pagination.pageIndex + 1,
                count: table.getPageCount(),
              })}
            </span>
            <div className="inline-controls">
              <Button
                size="icon"
                disabled={!table.getCanPreviousPage()}
                onClick={() => table.previousPage()}
                aria-label={t("previousPage")}
              >
                <ChevronLeft className="directional" />
              </Button>
              <Button
                size="icon"
                disabled={!table.getCanNextPage()}
                onClick={() => table.nextPage()}
                aria-label={t("nextPage")}
              >
                <ChevronRight className="directional" />
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
