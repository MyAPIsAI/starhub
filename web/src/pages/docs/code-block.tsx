import { createContext, useContext } from "react";
import { useTranslation } from "react-i18next";
import { Copy } from "lucide-react";
import { copyText } from "../../features/code-example";
import { placeholders } from "../../config/placeholders";

export const DocsCodeContext = createContext({ apiBase: "", baseValid: false });

export function CodeBlock({ title, code }: { title: string; code: string }) {
  const { t } = useTranslation();
  const { apiBase, baseValid } = useContext(DocsCodeContext);
  const rendered = code.replaceAll(placeholders.apiBase, apiBase);
  return (
    <div className="docs-code">
      <div>
        <span>{title}</span>
        <button
          className="button button-small button-ghost"
          disabled={!baseValid}
          onClick={() => copyText(rendered, t("copied"), t("copyFailed"))}
        >
          <Copy size={14} />
          {t("copy")}
        </button>
      </div>
      <pre dir="ltr" tabIndex={0}>
        <code>{rendered}</code>
      </pre>
    </div>
  );
}
