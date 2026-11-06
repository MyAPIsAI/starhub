import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { Button } from "../components/ui/button";
import { useOptionStatus, useReloadOptions } from "../lib/api/option";
import { ROLE_ADMIN, ROLE_ROOT, useSystemAccess } from "../lib/system-access";
import "./configuration-sync.css";

export function ConfigurationSync() {
  const { i18n } = useTranslation();
  const text = (zh: string, en: string, ar: string) =>
    i18n.language === "zh" ? zh : i18n.language === "ar" ? ar : en;
  const role = useSystemAccess((s) => s.role);
  const query = useOptionStatus(5000);
  const reload = useReloadOptions();
  if (role !== ROLE_ADMIN && role !== ROLE_ROOT) return null;
  const data = query.error ? undefined : query.data;
  const date = (timestamp: number) =>
    new Intl.DateTimeFormat(regionalLocale(i18n.language), {
      dateStyle: "short",
      timeStyle: "medium",
      timeZone: "UTC",
    }).format(new Date(timestamp * 1000));
  return (
    <section className="configuration-sync">
      <header>
        <div>
          <h2>
            {text(
              ...translatedMessage("configuration_synchronization_b4b31c40e6"),
            )}
          </h2>
          <p>
            {text(
              ...translatedMessage(
                "reload_applies_to_the_process_serving_this_re_3c0d4856a1",
              ),
            )}
          </p>
        </div>
        <div className="configuration-sync-actions">
          <Button
            variant="secondary"
            disabled={query.isFetching}
            onClick={() => void query.refetch()}
          >
            {text(...translatedMessage("refresh_status_df30c1cf9e"))}
          </Button>
          <Button disabled={reload.isPending} onClick={() => reload.mutate()}>
            <RefreshCw size={15} />
            {reload.isPending
              ? text(...translatedMessage("reloading_9600c93ad7"))
              : text(...translatedMessage("reload_serving_process_fe504ada26"))}
          </Button>
        </div>
      </header>
      {query.isLoading && (
        <p role="status">
          {text(
            ...translatedMessage("loading_configuration_status_109e14a0e7"),
          )}
        </p>
      )}
      {query.error && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "configuration_status_is_unavailable_retry_lat_3776a46719",
            ),
          )}
        </p>
      )}
      {reload.error && (
        <p role="alert">
          {text(
            ...translatedMessage(
              "the_reload_result_could_not_be_confirmed_refr_7b6e1876c2",
            ),
          )}
        </p>
      )}
      {reload.data && !reload.error && (
        <p className="configuration-sync-notice" role="status">
          {text(
            ...translatedMessage(
              "last_reload_response_process_loaded_v_databas_43630b262e",
              {
                p0: reload.data.loaded_version,
                p1: reload.data.database_version,
              },
            ),
          )}
        </p>
      )}
      {data && (
        <>
          <dl className="configuration-sync-versions">
            <div>
              <dt>
                {text(...translatedMessage("database_version_7331c145d8"))}
              </dt>
              <dd>v{data.database_version}</dd>
            </div>
            <div>
              <dt>
                {text(
                  ...translatedMessage(
                    "serving_process_loaded_version_2a52048ae9",
                  ),
                )}
              </dt>
              <dd>v{data.loaded_version}</dd>
              <small data-local-sync={data.in_sync ? "current" : "different"}>
                {data.in_sync
                  ? text(
                      ...translatedMessage(
                        "serving_process_matches_database_b7fbb0d965",
                      ),
                    )
                  : text(
                      ...translatedMessage(
                        "serving_process_has_a_different_version_0fecefe6e9",
                      ),
                    )}
              </small>
            </div>
          </dl>
          {data.instances.length === 0 ? (
            <p className="configuration-sync-empty">
              {text(
                ...translatedMessage(
                  "no_active_instance_heartbeats_overall_synchro_41913a1f2b",
                ),
              )}
            </p>
          ) : (
            <div
              className="configuration-sync-table"
              tabIndex={0}
              role="region"
              aria-label={text(
                ...translatedMessage(
                  "instance_configuration_heartbeats_eb3112553a",
                ),
              )}
            >
              <table>
                <thead>
                  <tr>
                    <th>
                      {text(...translatedMessage("instance_id_971e658e35"))}
                    </th>
                    <th>
                      {text(
                        ...translatedMessage("reported_version_e6c2c4a3fb"),
                      )}
                    </th>
                    <th>
                      {text(
                        ...translatedMessage("version_comparison_4463c6e733"),
                      )}
                    </th>
                    <th>
                      {text(
                        ...translatedMessage("last_heartbeat_utc_ef8bf50fd2"),
                      )}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.instances.map((instance) => (
                    <tr key={instance.id}>
                      <td>
                        <code>{instance.id}</code>
                      </td>
                      <td>v{instance.version}</td>
                      <td
                        data-peer-sync={
                          instance.version === data.database_version
                            ? "current"
                            : "different"
                        }
                      >
                        {instance.version === data.database_version
                          ? text(
                              ...translatedMessage(
                                "matches_database_bd5bac129d",
                              ),
                            )
                          : text(
                              ...translatedMessage(
                                "different_version_f030486034",
                              ),
                            )}
                      </td>
                      <td>{date(instance.last_seen)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </section>
  );
}
