import { messagePair } from "../i18n/messages";
import { useQuery } from "@tanstack/react-query";
import { Button } from "../components/ui/button";
import { auditSummary } from "../lib/audit-display";
import { useStaffText } from "../lib/api/staff";
import { apiGet } from "../lib/api/http";
import { useSessionQueryIdentity } from "../lib/system-access";
import {
  AdminFeedback,
  AdminTable,
  adminDate,
  useAdminText,
} from "./admin-shared";

export function ChangeSummary({ value }: { value: string }) {
  const text = useStaffText();
  const description = auditSummary(value, text);
  return description.length > 160 ? (
    <details className="adm-change-summary">
      <summary>{description.slice(0, 140)}…</summary>
      {description}
    </details>
  ) : (
    <span className="adm-change-summary">{description}</span>
  );
}
export { AdminAudit } from "./admin-audit";

interface PriceChange {
  id: number;
  user_id: number;
  created_at: number;
  action: string;
  object_id: string;
  before_summary: string;
  after_summary: string;
  username: string;
}
export function PricingHistory({
  kind,
  id,
}: {
  kind: "models" | "service-group";
  id: string;
}) {
  const t = useAdminText(),
    identity = useSessionQueryIdentity();
  const query = useQuery({
    queryKey: ["pricing-history", ...identity, kind, id],
    queryFn: () =>
      apiGet<PriceChange[]>(
        `/api/admin/pricing-history?kind=${kind}&object=${encodeURIComponent(id)}`,
      ),
    retry: false,
  });
  return (
    <div className="adm-page">
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {query.data?.map((h) => (
        <article className="adm-card" key={h.id}>
          <strong>
            {adminDate(h.created_at)} · {h.username}
          </strong>
          <p>{t(...messagePair("before_761653b175"))}</p>
          <ChangeSummary value={h.before_summary} />
          <p>{t(...messagePair("after_d9e2277599"))}</p>
          <ChangeSummary value={h.after_summary} />
        </article>
      ))}
      {query.data?.length === 0 && (
        <p>{t(...messagePair("no_recorded_pricing_changes_3f0875ac46"))}</p>
      )}
    </div>
  );
}

export { AdminReconciliation } from "./admin-reconciliation";

interface SystemInfo {
  version: string;
  go_version: string;
  timezone: string;
  instances: {
    id: string;
    version: string;
    configuration_version: number;
    last_seen: number;
    status: string;
    health_checks: Record<string, boolean>;
  }[];
}
export function AdminSystem() {
  const t = useAdminText(),
    identity = useSessionQueryIdentity();
  const query = useQuery({
    queryKey: ["staff-system", ...identity],
    queryFn: () => apiGet<SystemInfo>("/api/admin/system"),
    retry: false,
    refetchInterval: 10000,
  });
  return (
    <section className="adm-page">
      <header className="adm-heading">
        <div>
          <h1>{t(...messagePair("system_information_19bdc9e4c3"))}</h1>
          <p>
            {t(
              ...messagePair(
                "actual_instance_versions_and_recent_health_ch_10db1593dc",
              ),
            )}
          </p>
        </div>
        <Button onClick={() => void query.refetch()}>
          {t(...messagePair("refresh_b1cae2bbd8"))}
        </Button>
      </header>
      <AdminFeedback
        pending={query.isPending}
        error={query.error}
        retry={() => void query.refetch()}
      />
      {query.data && (
        <>
          <div className="adm-card">
            {query.data.version} · {query.data.go_version} ·{" "}
            {query.data.timezone}
          </div>
          <AdminTable label={t(...messagePair("running_instances_000e5bede8"))}>
            <thead>
              <tr>
                {[
                  t(...messagePair("instance_8f5d025b3a")),
                  t(...messagePair("version_85d3816499")),
                  t(...messagePair("configuration_6bd473ecde")),
                  t(...messagePair("last_heartbeat_d2d91a9e3c")),
                  t(...messagePair("health_checks_c4399cfa58")),
                ].map((c) => (
                  <th key={c}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {query.data.instances.map((i) => (
                <tr key={i.id}>
                  <td>{i.id}</td>
                  <td>
                    {i.version || t(...messagePair("not_reported_b165492e89"))}
                  </td>
                  <td>{i.configuration_version}</td>
                  <td>{adminDate(i.last_seen)}</td>
                  <td>
                    {i.status}
                    <small>
                      {Object.entries(i.health_checks)
                        .map(([name, ok]) => `${name}: ${ok ? "✓" : "×"}`)
                        .join(" · ")}
                    </small>
                  </td>
                </tr>
              ))}
            </tbody>
          </AdminTable>
        </>
      )}
    </section>
  );
}
