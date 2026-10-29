import { NativeKeyForm } from "./native-key-form";
import type { TierId } from "../config/placeholders";
export default function CreateKey({
  open,
  setOpen,
  initialModel,
  initialGroup,
  onCreated,
}: {
  open: boolean;
  setOpen: (value: boolean) => void;
  initialModel?: string;
  initialGroup?: TierId;
  onCreated?: (id: string, key: string) => void;
}) {
  return open ? (
    <NativeKeyForm
      onClose={() => setOpen(false)}
      initialModel={initialModel}
      initialGroup={initialGroup}
      onCreated={(token) => onCreated?.(String(token.id), token.key)}
    />
  ) : null;
}
