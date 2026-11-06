import { message as translatedMessage } from "../i18n/messages";
import { Link } from "react-router-dom";
import { useSettingsText } from "./settings-text";
import "./channel-routing-notice.css";

export function ChannelRoutingNotice({
  onOpenBindings,
}: {
  onOpenBindings: () => void;
}) {
  const { text } = useSettingsText();
  return (
    <aside
      className="channel-routing-notice"
      aria-label={text(
        ...translatedMessage("routing_effects_for_managed_channels_b7094d99ce"),
      )}
    >
      <strong>
        {text(
          ...translatedMessage(
            "routing_effects_for_managed_channels_b7094d99ce",
          ),
        )}
      </strong>
      <p>
        {text(
          ...translatedMessage(
            "ordinary_channels_use_the_models_and_groups_c_42cc5000c0",
          ),
        )}
      </p>
      <p>
        {text(
          ...translatedMessage(
            "changing_a_managed_channel_s_key_upstream_add_24612b08a1",
          ),
        )}
      </p>
      <Link to="/console/channels#gateway-bindings" onClick={onOpenBindings}>
        {text(
          ...translatedMessage(
            "leave_editor_and_view_binding_state_b8f2baa0b6",
          ),
        )}
      </Link>
      <p className="channel-routing-notice-hint">
        {text(
          ...translatedMessage(
            "navigation_does_not_submit_unsaved_form_chang_bafc119a1d",
          ),
        )}
      </p>
    </aside>
  );
}
