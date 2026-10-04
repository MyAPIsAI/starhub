import { ArrowRight, KeyRound, CircleDollarSign, PlugZap } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { usePreferences } from "../lib/preferences";
import { useProductPreview } from "../lib/product-preview";
import { useSystemAccess, systemRoleForScenario } from "../lib/system-access";
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
    path: "/console/model-pricing",
    Icon: CircleDollarSign,
    title: { zh: "运营改价", en: "Pricing operations", ar: "إدارة الأسعار" },
    body: {
      zh: "预览客户影响 → 发布演示价格 → 新旧调用费用对照。",
      en: "Preview customer impact, publish demo rates and compare old and new calls.",
      ar: "عاين الأثر وانشر أسعاراً تجريبية وقارن الطلبات السابقة والجديدة.",
    },
  },
] as const;

export function DemoScenarios({ onSelect }: { onSelect?: () => void }) {
  const language = usePreferences((s) => s.language);
  const resetScenario = useProductPreview((s) => s.resetScenario);
  const navigate = useNavigate();
  return (
    <div className="demo-scenario-list">
      <p className="demo-scenario-note">
        {language === "zh"
          ? "选择场景会重置当前浏览器的演示业务与价格。场景中可自由操作，随时通过右下角入口重置。"
          : language === "ar"
            ? "اختيار سيناريو يعيد بيانات العرض والأسعار في هذا المتصفح. يمكنك العمل بحرية وإعادة الضبط من زر العرض."
            : "Selecting a scenario resets this browser's demo data and prices. Explore freely and reset from the demo control."}
      </p>
      {demoScenarios.map(({ id, path, Icon, title, body }) => (
        <button
          key={id}
          className="demo-scenario-card"
          onClick={() => {
            useSystemAccess.getState().setRole(systemRoleForScenario(id));
            resetScenario(id);
            onSelect?.();
            navigate(path);
          }}
        >
          <Icon size={22} />
          <span>
            <strong>{title[language]}</strong>
            <small>{body[language]}</small>
          </span>
          <ArrowRight size={18} className="directional" />
        </button>
      ))}
    </div>
  );
}
