import { message as translatedMessage } from "../i18n/messages";
import { useSettingsText } from "./settings-text";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Route } from "lucide-react";
import { useModelPricing } from "../lib/model-pricing";
import { useBackendKeys } from "../lib/api/key-adapter";
import { useBackendRequests } from "../lib/api/usage";
import { getKeyModels, resolveKeyModelResult } from "../lib/key-model-policy";
import { groupResources } from "../lib/platform-catalog";
import { useSystemAccess, isAdmin } from "../lib/system-access";
import { Button } from "../components/ui/button";
import { formatMoney } from "../lib/money";
import "../components/catalog-management.css";
export default function ResourceRouting() {
  const { text } = useSettingsText();
  const config = useModelPricing((s) => s.config),
    keys = useBackendKeys().keys,
    requests = useBackendRequests(20).rows,
    role = useSystemAccess((s) => s.role);
  const [keyId, setKeyId] = useState("");
  const key = keys.find((k) => k.id === keyId) ?? keys.find((k) => k.active);
  const models = key ? getKeyModels(key, config) : [];
  const smartRequests = requests.filter(
    (request) =>
      request.requested_model === "myapis-auto" ||
      request.requested_model === "veyra-auto",
  );
  const resolved = key
    ? resolveKeyModelResult(key, "veyra-auto", config, {
        protocol: "OpenAI",
        capability: "text",
        inputTokens: 1000,
        maxTokens: 500,
      })
    : null;
  return (
    <div className="catalog-management">
      <header className="cm-heading">
        <div>
          <span className="pricing-eyebrow">
            <Route size={16} />
            {text(...translatedMessage("ui_smart_routing_8483f2a7"))}
          </span>
          <h1>
            {text(
              ...translatedMessage(
                "ui_routing_candidates_and_request_traces_2a74fc56",
              ),
            )}
          </h1>
          <p>
            {text(
              ...translatedMessage(
                "ui_requests_for_myapis_auto_with_legacy_veyra_au_cfc56e6b",
              ),
            )}
          </p>
        </div>
        <Button asChild>
          <Link to="/console/playground">
            {text(...translatedMessage("ui_test_a_request_528928a2"))}
          </Link>
        </Button>
      </header>
      <section className="cm-callout">
        <label className="cm-actions">
          {text(...translatedMessage("ui_view_key_candidates_abe3d683"))}
          <select
            aria-label={text(
              ...translatedMessage("ui_select_a_routing_key_e58d390c"),
            )}
            value={key?.id ?? ""}
            onChange={(e) => setKeyId(e.target.value)}
          >
            {keys.map((k) => (
              <option key={k.id} value={k.id}>
                {k.name}
                {!k.active
                  ? text(...translatedMessage("ui__disabled__fadaeaa9"))
                  : ""}
              </option>
            ))}
          </select>
        </label>
        <p>
          {text(
            ...translatedMessage(
              "ui_estimate_openai_compatible_text_protocol_1_00_01339517",
            ),
          )}
        </p>
        <p>
          {!key
            ? text(...translatedMessage("ui_create_a_key_first__b063c1e6"))
            : resolved?.model
              ? text(
                  ...translatedMessage(
                    "ui_current_candidate_this_is_a_read_only_estimat_da594dbd",
                    { p0: resolved.model.name },
                  ),
                )
              : text(
                  ...translatedMessage(
                    "ui_cannot_route_check_group_access_smart_routing_9e86a483",
                    {
                      p0:
                        resolved?.error ??
                        text(...translatedMessage("ui_no_candidates_070a0c0a")),
                    },
                  ),
                )}
        </p>
        <small>
          {text(
            ...translatedMessage(
              "ui_disabled_keys_or_insufficient_balance_or_quot_67485bb9",
            ),
          )}
        </small>
      </section>
      <div className="cm-table">
        <table>
          <thead>
            <tr>
              <th>
                {text(
                  ...translatedMessage(
                    "ui_authorized_standard_models_fc483283",
                  ),
                )}
              </th>
              <th>
                {text(...translatedMessage("ui_supported_protocols_a3c15a59"))}
              </th>
              <th>
                {text(...translatedMessage("ui_available_instances_bff9b979"))}
              </th>
              <th>{text(...translatedMessage("status_dda39dcd63"))}</th>
            </tr>
          </thead>
          <tbody>
            {models.map((m) => (
              <tr key={m.id}>
                <td>
                  {m.name}
                  <code>{m.id}</code>
                </td>
                <td>
                  {m.protocols.join(" / ") ||
                    text(...translatedMessage("ui_media_billing_8e5fdbc9"))}
                </td>
                <td>{key && groupResources(config, key.tier, m.id).length}</td>
                <td>
                  {resolved?.model?.id === m.id
                    ? text(
                        ...translatedMessage(
                          "ui_current_estimated_selection_073f8f40",
                        ),
                      )
                    : text(
                        ...translatedMessage(
                          "ui_request_requirements_pending_7c76a84a",
                        ),
                      )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!models.length && (
          <p className="cm-empty">
            {text(
              ...translatedMessage(
                "ui_no_callable_models_are_available_for_this_key_c766e942",
              ),
            )}
          </p>
        )}
      </div>
      <section className="cm-callout">
        <h3>
          {text(
            ...translatedMessage("ui_recent_smart_routing_requests_b3d697f2"),
          )}
        </h3>
        {smartRequests.slice(0, 8).map((r) => (
          <p key={r.trace_id}>
            <Link
              to={`/console/requests?trace=${encodeURIComponent(r.trace_id)}`}
            >
              {r.trace_id}
            </Link>{" "}
            · {r.actual_model ?? r.error_message} ·{" "}
            {formatMoney(r.cost_nano, "call")}
          </p>
        ))}
        {!smartRequests.length && (
          <p>
            {text(
              ...translatedMessage(
                "ui_no_smart_routing_requests_yet_test_the_same_k_7be20bd0",
              ),
            )}
          </p>
        )}
      </section>
      {isAdmin(role) && (
        <details className="cm-callout">
          <summary>
            {text(
              ...translatedMessage(
                "ui_advanced_view_resource_snapshot_3deeaca3",
              ),
            )}
          </summary>
          <p>
            {text(
              ...translatedMessage(
                "ui_snapshot_shared_with_channel_management_f87c9ce7",
              ),
            )}
            <code>{config.resources?.id}</code>
            {text(...translatedMessage("ui__current_count__f2d2b631"))}{" "}
            {config.resources?.instances.length}{" "}
            {text(
              ...translatedMessage(
                "ui_instances_no_separate_resource_or_route_recor_e7734e9b",
              ),
            )}
          </p>
          <p>
            {text(
              ...translatedMessage(
                "ui_resources_come_from_a_local_simulation_of_the_512a7061",
              ),
            )}
          </p>
          <Button asChild>
            <Link to="/console/channels">
              {text(
                ...translatedMessage(
                  "ui_inspect_channels_and_resource_placement_383f0b17",
                ),
              )}
            </Link>
          </Button>
        </details>
      )}
    </div>
  );
}
