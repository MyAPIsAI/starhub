import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { PublicHeader } from "../../components/product-shell";
import { FinanceOverview } from "../../features/finance-overview";

export default function FinancePortal() {
  const { t } = useTranslation();
  return (
    <div className="brand-home public-refined">
      <PublicHeader />
      <main className="finance-portal-main">
        <Link className="finance-portal-back" to="/console">
          ← {t("pConsole")}
        </Link>
        <FinanceOverview />
      </main>
    </div>
  );
}
