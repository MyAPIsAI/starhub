import { useState } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import {
  SlidersHorizontal,
  ChevronDown,
  ChevronUp,
  RotateCcw,
} from "lucide-react";
import { useProductPreview } from "../lib/product-preview";
import { usePreferences } from "../lib/preferences";
import { DemoScenarios, demoScenarios } from "./demo-scenarios";
import { useSystemAccess, systemRoleForScenario } from "../lib/system-access";

export function DemoControlBar({ inline = false }: { inline?: boolean }) {
  const language = usePreferences((s) => s.language);
  const text = (zh: string, en: string, ar: string) =>
    language === "zh" ? zh : language === "ar" ? ar : en;
  const navigate = useNavigate();
  const location = useLocation();
  const [expanded, setExpanded] = useState(false);
  const mode = useProductPreview((s) => s.demoMode);
  const scenario = useProductPreview((s) => s.scenario);
  const { role, setRole } = useSystemAccess();
  const resetScenario = useProductPreview((s) => s.resetScenario);
  const setBalance = useProductPreview((s) => s.demoSetBalanceState);
  const setOutage = useProductPreview((s) => s.demoSetUpstreamOutage);
  const setData = useProductPreview((s) => s.demoSetDataState);
  const quickPages = [
    [text("接入中心", "Connect", "الاتصال"), "/console/connect"],
    [text("密钥", "Keys", "المفاتيح"), "/console/keys"],
    [text("验证配置", "Verify", "التحقق"), "/console/playground"],
    [text("调用记录", "Requests", "الطلبات"), "/console/requests"],
    [text("账务", "Billing", "الفوترة"), "/console/billing"],
    ...(role === "superAdmin"
      ? [
          [
            text("标准模型", "Standard models", "النماذج القياسية"),
            "/console/model-pricing",
          ],
          [text("渠道管理", "Channels", "القنوات"), "/console/channels"],
          [
            text("服务分组", "Service groups", "مجموعات الخدمة"),
            "/console/service-groups",
          ],
          [text("管理后台", "Admin portal", "بوابة الإدارة"), "/admin"],
        ]
      : []),
  ];
  // These workspaces provide an inline footer to keep row actions unobstructed.
  if (
    !inline &&
    ((role === "superAdmin" && /\/channels\/?$/.test(location.pathname)) ||
      /\/(requests|service-quality)\/?$/.test(location.pathname) ||
      location.pathname === "/models")
  )
    return null;
  return (
    <div className={`demo-controls${inline ? " demo-controls-inline" : ""}`}>
      <button
        onClick={() => setExpanded((value) => !value)}
        aria-expanded={expanded}
        aria-controls="demo-controls-panel"
      >
        <SlidersHorizontal size={14} />
        {text(
          "演示场景与重置",
          "Demo scenarios & reset",
          "سيناريوهات العرض وإعادة الضبط",
        )}
        {expanded ? <ChevronDown size={14} /> : <ChevronUp size={14} />}
      </button>
      {expanded && (
        <div className="demo-controls-panel" id="demo-controls-panel">
          <DemoScenarios onSelect={() => setExpanded(false)} />
          <button
            className="demo-control-reset"
            onClick={() => {
              const current =
                demoScenarios.find((item) => item.id === scenario) ??
                demoScenarios[0];
              resetScenario(current.id);
              setRole(systemRoleForScenario(current.id));
              navigate(current.path);
              setExpanded(false);
            }}
          >
            <RotateCcw size={14} />
            {text(
              "重置当前场景",
              "Reset current scenario",
              "إعادة ضبط السيناريو الحالي",
            )}
          </button>
          <div className="demo-control-group">
            <strong>
              {text(
                "演示账户身份",
                "Demo account role",
                "هوية الحساب التجريبي",
              )}
            </strong>
            <div className="demo-control-options">
              <button
                aria-pressed={role === "customer"}
                onClick={() => setRole("customer")}
              >
                {text("普通用户", "Customer", "مستخدم")}
              </button>
              <button
                aria-pressed={role === "superAdmin"}
                onClick={() => setRole("superAdmin")}
              >
                {text("系统管理员", "System administrator", "مسؤول النظام")}
              </button>
            </div>
            <small>
              {text(
                "仅切换本地演示权限，未接入真实身份认证。",
                "Local demo permissions only; no live authentication.",
                "صلاحيات عرض محلية فقط دون مصادقة حقيقية.",
              )}
            </small>
          </div>
          <details>
            <summary>
              {text(
                "高级演示状态",
                "Advanced demo states",
                "حالات العرض المتقدمة",
              )}
            </summary>
            <div className="demo-control-group" style={{ marginTop: 12 }}>
              <strong>
                {text("模拟余额", "Simulated balance", "الرصيد المحاكى")}
              </strong>
              <div className="demo-control-options">
                {(
                  [
                    ["normal", text("充足", "Funded", "كافٍ")],
                    ["low", text("偏低", "Low", "منخفض")],
                    ["zero", text("耗尽", "Empty", "فارغ")],
                  ] as const
                ).map(([id, label]) => (
                  <button
                    key={id}
                    onClick={() => setBalance(id)}
                    aria-pressed={mode.balanceState === id}
                  >
                    {label}
                  </button>
                ))}
              </div>
              <strong>
                {text(
                  "模拟上游状态",
                  "Simulated upstream",
                  "حالة المزوّد المحاكاة",
                )}
              </strong>
              <div className="demo-control-options">
                <button
                  aria-pressed={mode.upstreamOutage}
                  onClick={() => setOutage(!mode.upstreamOutage)}
                >
                  {mode.upstreamOutage
                    ? text(
                        "故障中 · 点击恢复",
                        "Outage · click to restore",
                        "عطل · انقر للاستعادة",
                      )
                    : text(
                        "正常 · 点击模拟故障",
                        "Healthy · simulate outage",
                        "سليم · محاكاة عطل",
                      )}
                </button>
              </div>
              <strong>{text("页面状态", "Page state", "حالة الصفحة")}</strong>
              <div className="demo-control-options">
                {(
                  [
                    ["normal", text("正常", "Ready", "جاهز")],
                    ["loading", text("加载", "Loading", "تحميل")],
                    ["empty", text("空白", "Empty", "فارغ")],
                    ["error", text("错误", "Error", "خطأ")],
                  ] as const
                ).map(([id, label]) => (
                  <button
                    key={id}
                    aria-pressed={mode.uiState === id}
                    onClick={() => setData(id)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
          </details>
          <div className="demo-control-group">
            <strong>
              {text("快速前往", "Quick navigation", "انتقال سريع")}
            </strong>
            <div className="demo-control-options">
              {quickPages.map(([label, path]) => (
                <button
                  key={path}
                  aria-pressed={location.pathname + location.search === path}
                  onClick={() => {
                    navigate(path);
                    setExpanded(false);
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
