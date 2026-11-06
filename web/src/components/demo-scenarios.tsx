import { localizedMessage } from "../i18n/messages";
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
    title: { zh: "首次接入", en: "First connection", ar: "الاتصال الأول" },
    body: {
      zh: "创建密钥 → 获取工具配置 → 切换模型验证 → 查看费用。",
      en: "Create a key, configure your tool, switch models and inspect costs.",
      ar: "أنشئ مفتاحاً واضبط أداتك وبدّل النماذج وتحقق من التكلفة.",
    },
  },
  {
    id: "enterprise",
    path: "/console/keys",
    Icon: KeyRound,
    title: {
      zh: "多用途密钥管理",
      en: "Keys for different uses",
      ar: "إدارة المفاتيح لأغراض متعددة",
    },
    body: {
      zh: "开发与生产分开管理，验证白名单、额度与智能路由边界。",
      en: "Separate development and production; verify allowlists, budgets and routing.",
      ar: "افصل التطوير والإنتاج واختبر الصلاحيات والميزانية والتوجيه.",
    },
  },
  {
    id: "pricing",
    path: "/console/model-pricing?demo=1",
    Icon: CircleDollarSign,
    title: { zh: "运营改价", en: "Pricing operations", ar: "إدارة الأسعار" },
    body: {
      zh: "管理员演示定价入口。保存仅影响此浏览器的模拟价格；场景不授予管理权限。",
      en: "Administrator Demo pricing entry. Saving affects simulated prices in this browser; scenarios do not grant management access.",
      ar: "مدخل تسعير تجريبي للمسؤول. يؤثر الحفظ في أسعار المحاكاة بهذا المتصفح؛ ولا تمنح السيناريوهات صلاحيات إدارية.",
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
