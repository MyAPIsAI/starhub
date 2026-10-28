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
            {text("系统设置", "System settings", "إعدادات النظام")}
          </span>
          <h1>{text("渠道管理", "Channels", "القنوات")}</h1>
          <p>
            {text(
              "按渠道或标准模型组织资源，统一管理映射与销售投放。",
              "Browse by channel or standard model. Manage resource mapping and placement.",
              "تصفح حسب القناة أو النموذج وأدر الربط والنشر.",
            )}
          </p>
        </div>
        <div className="channel-page-actions">
          <Button asChild>
            <Link to="/console/model-pricing">
              {text("标准模型", "Standard models", "النماذج القياسية")}
            </Link>
          </Button>
          <Button variant="primary" onClick={() => setOpen(true)}>
            <RefreshCw size={15} />
            {text("同步资源", "Sync resources", "مزامنة الموارد")}
          </Button>
        </div>
      </header>
      <div className="channel-source-strip">
        <div>
          <Database size={14} />
          <strong>
            {text("资源管理平台", "Resource platform", "منصة الموارد")}
          </strong>
          <span className="channel-demo-label">
            {text("本地模拟", "Local demo", "عرض محلي")}
          </span>
          <code>{snapshot.id}</code>
          <span className="channel-source-time">
            {text("更新于", "Updated", "تحديث")}{" "}
            {new Date(snapshot.receivedAt).toLocaleString(
              lang === "zh" ? "zh-CN" : lang === "ar" ? "ar" : "en-US",
            )}
          </span>
        </div>
        <button onClick={() => setOpen(true)}>
          {text("快照详情", "Snapshot details", "تفاصيل اللقطة")}
          <ChevronRight size={13} />
        </button>
      </div>
      <Modal
        open={open}
        onOpenChange={setOpen}
        title={text("同步资源", "Sync resources", "مزامنة الموارد")}
        description={text(
          "使用本地快照模拟资源管理平台。同步前先检查差异，不自动开放新资源。",
          "Local snapshots simulate the resource platform. Review changes before syncing; new resources are never opened automatically.",
          "تحاكي اللقطات المحلية المنصة. راجع التغييرات؛ لا تنشر الموارد الجديدة تلقائياً.",
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
              {text("导入快照数据", "Import snapshot", "استيراد لقطة")}
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                setOpen(false);
                if (mode === "fail")
                  onFailure(
                    text(
                      "模拟同步失败：资源平台暂时不可用。继续使用现有快照，本次未改变资源状态或本地投放。",
                      "Demo sync failed: the resource platform is unavailable. The existing snapshot and placements are unchanged.",
                      "فشلت المزامنة المحاكاة. بقيت اللقطة وإعدادات النشر كما هي.",
                    ),
                  );
                else onPreview(nextDemoSnapshot(config, mode));
              }}
            >
              <RefreshCw size={14} />
              {text("预览模拟同步", "Preview demo sync", "معاينة المزامنة")}
            </Button>
          </>
        }
      >
        <div className="channel-sync-status">
          <Database size={22} />
          <div>
            <strong>
              {text(
                "资源管理平台 · 本地快照",
                "Resource platform · local snapshot",
                "منصة الموارد · لقطة محلية",
              )}
            </strong>
            <p>
              <code>{snapshot.id}</code> · {snapshot.channels.length}{" "}
              {text("个渠道", "channels", "قنوات")} ·{" "}
              {snapshot.instances.length} {text("个实例", "instances", "موارد")}
            </p>
          </div>
        </div>
        <div className="cm-form">
          <label className="cm-wide">
            {text("同步演示场景", "Demo sync scenario", "سيناريو المزامنة")}
            <select
              aria-label={text(
                "同步演示场景",
                "Demo sync scenario",
                "سيناريو المزامنة",
              )}
              value={mode}
              onChange={(e) => setMode(e.target.value as typeof mode)}
            >
              <option value="add">
                {text(
                  "发现一个新资源",
                  "Discover a new resource",
                  "اكتشاف مورد جديد",
                )}
              </option>
              <option value="outage">
                {text(
                  "模拟上游资源不可用",
                  "Simulate source outage",
                  "محاكاة تعطل المصدر",
                )}
              </option>
              <option value="restore">
                {text(
                  "恢复资源可用",
                  "Restore resource availability",
                  "استعادة الموارد",
                )}
              </option>
              <option value="fail">
                {text(
                  "模拟同步失败",
                  "Simulate sync failure",
                  "محاكاة فشل المزامنة",
                )}
              </option>
            </select>
          </label>
        </div>
        <details className="channel-sync-meta">
          <summary>
            {text(
              "来源与快照信息",
              "Source and snapshot information",
              "معلومات المصدر واللقطة",
            )}
          </summary>
          <dl className="cm-details">
            <div>
              <dt>{text("源发布时间", "Published at", "وقت النشر")}</dt>
              <dd>
                {new Date(snapshot.publishedAt).toLocaleString(
                  lang === "zh" ? "zh-CN" : lang === "ar" ? "ar" : "en-US",
                )}
              </dd>
            </div>
            <div>
              <dt>{text("本地接收时间", "Received at", "وقت الاستلام")}</dt>
              <dd>
                {new Date(snapshot.receivedAt).toLocaleString(
                  lang === "zh" ? "zh-CN" : lang === "ar" ? "ar" : "en-US",
                )}
              </dd>
            </div>
            <div>
              <dt>{text("快照序号", "Snapshot sequence", "تسلسل اللقطة")}</dt>
              <dd>{snapshot.sequence}</dd>
            </div>
          </dl>
          <p>
            {text(
              "渠道、上游模型、能力与状态是只读来源字段。本地模型映射及销售投放独立保存；智能路由共用此快照。",
              "Source fields are read-only. Local mappings and placement are saved separately; routing shares this snapshot.",
              "حقول المصدر للقراءة فقط. يحفظ الربط والنشر محلياً وتستخدم اللقطة نفسها للتوجيه.",
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
        "来源已移除的实例",
        "Resources removed from source",
        "الموارد المحذوفة من المصدر",
      )}
      description={text(
        "已停止进入调用候选。本地映射和历史仍保留，可检查后清除投放。",
        "These resources no longer serve requests. Local mapping and history are retained; review before clearing placement.",
        "لا تدخل هذه الموارد في الطلبات. يبقى الربط والسجل ويمكن مراجعتها قبل مسح النشر.",
      )}
      footer={
        <Button onClick={() => onOpenChange(false)}>
          {text("关闭", "Close", "إغلاق")}
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
                  text("未映射", "Unmapped", "غير مرتبط")}
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
                "检查并清除投放",
                "Review and clear placement",
                "مراجعة ومسح النشر",
              )}
            </Button>
          </div>
        ))}
        {!missing.length && (
          <p className="cm-empty">
            {text(
              "没有已移除来源的本地配置。",
              "No orphaned local placements.",
              "لا توجد إعدادات محلية بلا مصدر.",
            )}
          </p>
        )}
      </div>
    </Modal>
  );
}
