import {
  message as translatedMessage,
  localizedMessage,
} from "../i18n/messages";

import { ArrowRight, KeyRound, CircleDollarSign, PlugZap } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { usePreferences } from "../lib/preferences";
import { writtenLanguage } from "../lib/localize";
import { useProductPreview } from "../lib/product-preview";
import { isAdmin, useSystemAccess } from "../lib/system-access";
import "./demo-scenarios.css";

export const demoScenarios = [
  {
    id: "first",
    path: "/console/connect",
    Icon: PlugZap,
    title: {
      zh: translatedMessage("ui_first_connection_65bf0869")[0],
      en: translatedMessage("ui_first_connection_65bf0869")[1],
      ar: translatedMessage("ui_first_connection_65bf0869")[2],
    },
    body: {
      zh: translatedMessage(
        "ui_create_a_key_configure_your_tool_switch_model_b994a51e",
      )[0],
      en: translatedMessage(
        "ui_create_a_key_configure_your_tool_switch_model_b994a51e",
      )[1],
      ar: translatedMessage(
        "ui_create_a_key_configure_your_tool_switch_model_b994a51e",
      )[2],
    },
  },
  {
    id: "enterprise",
    path: "/console/keys",
    Icon: KeyRound,
    title: {
      zh: translatedMessage("ui_keys_for_different_uses_6809a050")[0],
      en: translatedMessage("ui_keys_for_different_uses_6809a050")[1],
      ar: translatedMessage("ui_keys_for_different_uses_6809a050")[2],
    },
    body: {
      zh: translatedMessage(
        "ui_separate_development_and_production_verify_al_adbb1de4",
      )[0],
      en: translatedMessage(
        "ui_separate_development_and_production_verify_al_adbb1de4",
      )[1],
      ar: translatedMessage(
        "ui_separate_development_and_production_verify_al_adbb1de4",
      )[2],
    },
  },
  {
    id: "pricing",
    path: "/console/model-pricing?demo=1",
    Icon: CircleDollarSign,
    title: {
      zh: translatedMessage("ui_pricing_operations_5e386fcb")[0],
      en: translatedMessage("ui_pricing_operations_5e386fcb")[1],
      ar: translatedMessage("ui_pricing_operations_5e386fcb")[2],
    },
    body: {
      zh: translatedMessage(
        "ui_administrator_demo_pricing_entry_saving_affec_650d7b7b",
      )[0],
      en: translatedMessage(
        "ui_administrator_demo_pricing_entry_saving_affec_650d7b7b",
      )[1],
      ar: translatedMessage(
        "ui_administrator_demo_pricing_entry_saving_affec_650d7b7b",
      )[2],
    },
  },
] as const;

export function DemoScenarios({ onSelect }: { onSelect?: () => void }) {
  const language = usePreferences((s) => s.language);
  const resetScenario = useProductPreview((s) => s.resetScenario);
  const navigate = useNavigate();
  const role = useSystemAccess((s) => s.role);
  const authenticated = useSystemAccess((s) => s.session.accountId !== null);
  return (
    <div className="demo-scenario-list">
      <p className="demo-scenario-note">
        {localizedMessage(
          "selecting_a_scenario_resets_local_demo_data_a_b8c9516e4a",
          language,
        )}
      </p>
      {demoScenarios.map(({ id, path, Icon, title, body }) => (
        <button
          key={id}
          className="demo-scenario-card"
          disabled={id === "pricing" && !isAdmin(role)}
          onClick={() => {
            resetScenario(id);
            onSelect?.();
            navigate(authenticated ? path : "/login");
          }}
        >
          <Icon size={22} />
          <span>
            <strong>{title[writtenLanguage(language)]}</strong>
            <small>{body[writtenLanguage(language)]}</small>
          </span>
          <ArrowRight size={18} className="directional" />
        </button>
      ))}
    </div>
  );
}
