import { NativeKeyForm } from "./native-key-form";
import type { PreviewKey } from "../lib/product-preview";
export default function EditKey({
  item,
  onClose,
}: {
  item: PreviewKey;
  onClose: () => void;
}) {
  return <NativeKeyForm item={item} onClose={onClose} />;
}
