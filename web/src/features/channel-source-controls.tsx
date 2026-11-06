import { regionalLocale } from "../lib/locale-format";
import { message as translatedMessage } from "../i18n/messages";
import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Database,
  RefreshCw,
  Upload,
  Network,
  ChevronRight,
} from "lucide-react";
import type { PricingCatalogConfig, ResourceSnapshot } from "../config/models";
import { nextDemoSnapshot } from "../lib/platform-catalog";
import { Button } from "../components/ui/button";
import { Modal } from "../components/ui/overlay";
import { useSettingsText } from "./settings-text";

export function ChannelSourceControls({
  config,
  onPreview,
  onImport,
  onFailure,
}: {
  config: PricingCatalogConfig;
  onPreview: (snapshot: ResourceSnapshot) => void;
  onImport: () => void;
  onFailure: (message: string) => void;
}) {
  const { text, lang } = useSettingsText();
  const snapshot = config.resources!;
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<"add" | "outage" | "restore" | "fail">(
    "add",
  );
  return (
    <>
      <header className="cm-heading">
        <div>
          <span className="pricing-eyebrow">
            <Network size={15} />
            {text(...translatedMessage("system_settings_966a9e1755"))}
          </span>
          <h1>{text(...translatedMessage("channels_eee07fd951"))}</h1>
          <p>
            {text(
              ...translatedMessage(
                "browse_by_channel_or_standard_model_manage_re_ff887719e4",
              ),
            )}
          </p>
        </div>
        <div className="channel-page-actions">
          <Button asChild>
            <Link to="/console/model-pricing">
              {text(...translatedMessage("standard_models_d7f00fd7f0"))}
            </Link>
          </Button>
          <Button variant="primary" onClick={() => setOpen(true)}>
            <RefreshCw size={15} />
            {text(...translatedMessage("sync_resources_99eef5158b"))}
          </Button>
        </div>
      </header>
      <div className="channel-source-strip">
        <div>
          <Database size={14} />
          <strong>
            {text(...translatedMessage("resource_platform_0c225d5c88"))}
          </strong>
          <span className="channel-demo-label">
            {text(...translatedMessage("local_demo_2768dd8aea"))}
          </span>
          <code>{snapshot.id}</code>
          <span className="channel-source-time">
            {text(...translatedMessage("updated_bf7485a3fd"))}{" "}
            {new Date(snapshot.receivedAt).toLocaleString(regionalLocale(lang))}
          </span>
        </div>
        <button onClick={() => setOpen(true)}>
          {text(...translatedMessage("snapshot_details_fcb75398b6"))}
          <ChevronRight size={13} />
        </button>
      </div>
      <Modal
        open={open}
        onOpenChange={setOpen}
        title={text(...translatedMessage("sync_resources_99eef5158b"))}
        description={text(
          ...translatedMessage(
            "local_snapshots_simulate_the_resource_platfor_5dccbf3786",
          ),
        )}
        footer={
          <>
            <Button
              onClick={() => {
                setOpen(false);
                onImport();
              }}
            >
              <Upload size={14} />
              {text(...translatedMessage("import_snapshot_694cc2cc87"))}
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                setOpen(false);
                if (mode === "fail")
                  onFailure(
                    text(
                      ...translatedMessage(
                        "demo_sync_failed_the_resource_platform_is_una_f681ff89ce",
                      ),
                    ),
                  );
                else onPreview(nextDemoSnapshot(config, mode));
              }}
            >
              <RefreshCw size={14} />
              {text(...translatedMessage("preview_demo_sync_60978ef001"))}
            </Button>
          </>
        }
      >
        <div className="channel-sync-status">
          <Database size={22} />
          <div>
            <strong>
              {text(
                ...translatedMessage(
                  "resource_platform_local_snapshot_a5576d22fd",
                ),
              )}
            </strong>
            <p>
              <code>{snapshot.id}</code> · {snapshot.channels.length}{" "}
              {text(...translatedMessage("channels_1d2c9778ad"))} ·{" "}
              {snapshot.instances.length}{" "}
              {text(...translatedMessage("instances_b8a43d724b"))}
            </p>
          </div>
        </div>
        <div className="cm-form">
          <label className="cm-wide">
            {text(...translatedMessage("demo_sync_scenario_780293076b"))}
            <select
              aria-label={text(
                ...translatedMessage("demo_sync_scenario_780293076b"),
              )}
              value={mode}
              onChange={(e) => setMode(e.target.value as typeof mode)}
            >
              <option value="add">
                {text(
                  ...translatedMessage("discover_a_new_resource_4486b88490"),
                )}
              </option>
              <option value="outage">
                {text(
                  ...translatedMessage("simulate_source_outage_8b5e639871"),
                )}
              </option>
              <option value="restore">
                {text(
                  ...translatedMessage(
                    "restore_resource_availability_b763bc8e9f",
                  ),
                )}
              </option>
              <option value="fail">
                {text(...translatedMessage("simulate_sync_failure_002325c1c7"))}
              </option>
            </select>
          </label>
        </div>
        <details className="channel-sync-meta">
          <summary>
            {text(
              ...translatedMessage(
                "source_and_snapshot_information_99fa9ba237",
              ),
            )}
          </summary>
          <dl className="cm-details">
            <div>
              <dt>{text(...translatedMessage("published_at_ac72dfa0c7"))}</dt>
              <dd>
                {new Date(snapshot.publishedAt).toLocaleString(
                  regionalLocale(lang),
                )}
              </dd>
            </div>
            <div>
              <dt>{text(...translatedMessage("received_at_496400eb28"))}</dt>
              <dd>
                {new Date(snapshot.receivedAt).toLocaleString(
                  regionalLocale(lang),
                )}
              </dd>
            </div>
            <div>
              <dt>
                {text(...translatedMessage("snapshot_sequence_804a2514e7"))}
              </dt>
              <dd>{snapshot.sequence}</dd>
            </div>
          </dl>
          <p>
            {text(
              ...translatedMessage(
                "source_fields_are_read_only_local_mappings_an_f1259e3110",
              ),
            )}
          </p>
        </details>
      </Modal>
    </>
  );
}
export function RemovedChannelResources({
  config,
  open,
  onOpenChange,
  onEdit,
}: {
  config: PricingCatalogConfig;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onEdit: (id: string) => void;
}) {
  const { text } = useSettingsText();
  const missing = Object.keys(config.placements ?? {}).filter(
    (id) => !config.resources?.instances.some((r) => r.id === id),
  );
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={text(
        ...translatedMessage("resources_removed_from_source_1561312c67"),
      )}
      description={text(
        ...translatedMessage(
          "these_resources_no_longer_serve_requests_loca_75a86de81b",
        ),
      )}
      footer={
        <Button onClick={() => onOpenChange(false)}>
          {text(...translatedMessage("close_6bf477426b"))}
        </Button>
      }
    >
      <div className="channel-removed-list">
        {missing.map((id) => (
          <div key={id}>
            <div>
              <code>{id}</code>
              <small>
                {config.placements?.[id]?.modelId ??
                  text(...translatedMessage("unmapped_6d198b7550"))}
              </small>
            </div>
            <Button
              size="small"
              onClick={() => {
                onOpenChange(false);
                onEdit(id);
              }}
            >
              {text(
                ...translatedMessage("review_and_clear_placement_1e1fedd015"),
              )}
            </Button>
          </div>
        ))}
        {!missing.length && (
          <p className="cm-empty">
            {text(
              ...translatedMessage("no_orphaned_local_placements_7f660e7504"),
            )}
          </p>
        )}
      </div>
    </Modal>
  );
}
