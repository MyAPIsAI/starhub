import { placeholders, type TierId } from "../config/placeholders";
import { usePreferences } from "../lib/preferences";
import { useModelPricing } from "../lib/model-pricing";
import { serviceGroupLabel } from "../lib/service-group-label";
export function TierBadge({ id }: { id: TierId }) {
  const language = usePreferences((s) => s.language);
  const config = useModelPricing((s) => s.config);
  const tier = placeholders.tiers.find((t) => t.id === id)!;
  const label = serviceGroupLabel(id, language, config);
  return (
    <span className="tier-badge">
      <span className="tier-mark" aria-hidden="true">
        {Array.from({ length: 4 }, (_, i) => (
          <span key={i} data-filled={i < (tier?.mark ?? 1)} />
        ))}
      </span>
      <span className="tier-label">
        {language === "zh" && label === tier?.name.zh ? <><span>{tier.name.en}</span><span className="tier-translation">{tier.name.zh.replace(tier.name.en, "")}</span></> : label}
      </span>
    </span>
  );
}
