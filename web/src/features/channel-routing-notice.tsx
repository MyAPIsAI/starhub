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
        "托管渠道的路由影响",
        "Routing effects for managed channels",
        "تأثيرات توجيه القنوات المُدارة",
      )}
    >
      <strong>
        {text(
          "托管渠道的路由影响",
          "Routing effects for managed channels",
          "تأثيرات توجيه القنوات المُدارة",
        )}
      </strong>
      <p>
        {text(
          "普通渠道使用这里配置的模型和分组；托管渠道以资源页明确发布的准确组合为准。修改此处的模型/分组不会扩展托管授权，也不会恢复已撤销或已释放的路线。",
          "Ordinary channels use the models and groups configured here. Managed channels use exact pairs explicitly published on the resource page. Editing these model/group fields does not expand managed grants or restore revoked or released routes.",
          "تستخدم القنوات العادية النماذج والمجموعات المحددة هنا. تستخدم القنوات المُدارة الأزواج الدقيقة المنشورة صراحةً في صفحة الموارد. لا يوسع تعديل هذه الحقول الصلاحيات المُدارة ولا يستعيد المسارات الملغاة أو المحررة.",
        )}
      </p>
      <p>
        {text(
          "修改托管渠道的密钥、上游地址或类型会撤销该渠道的全部托管路线。改回配置不会自动恢复；需要重新预览并确认发布。",
          "Changing a managed channel’s key, upstream address or type revokes all its managed routes. Restoring those settings does not reopen routes; a new review and explicit publication are required.",
          "يؤدي تغيير مفتاح القناة المُدارة أو عنوان المزود أو نوعها إلى إلغاء جميع مساراتها المُدارة. لا تعيد استعادة الإعدادات فتح المسارات؛ تلزم مراجعة ونشر صريحان جديدان.",
        )}
      </p>
      <Link to="/console/channels#gateway-bindings" onClick={onOpenBindings}>
        {text(
          "离开编辑并查看绑定状态",
          "Leave editor and view binding state",
          "مغادرة المحرر وعرض حالة الروابط",
        )}
      </Link>
      <p className="channel-routing-notice-hint">
        {text(
          "跳转不会提交未保存的表单。",
          "Navigation does not submit unsaved form changes.",
          "لا يؤدي الانتقال إلى إرسال تغييرات النموذج غير المحفوظة.",
        )}
      </p>
    </aside>
  );
}
