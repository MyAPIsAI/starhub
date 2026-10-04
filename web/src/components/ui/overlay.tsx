import { useRef, type ReactNode, type RefObject } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import * as AlertPrimitive from "@radix-ui/react-alert-dialog";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "./button";
import { useFocusReturn } from "./focus-return";
import "./modal.css";

/** Short, focused tasks. Long workflows belong on a routable page. */
export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  initialFocus,
  size = "default",
  className = "",
  preventOutsideClose = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  children: ReactNode;
  footer?: ReactNode;
  initialFocus?: RefObject<HTMLElement>;
  size?: "compact" | "default" | "wide";
  className?: string;
  preventOutsideClose?: boolean;
}) {
  const { t } = useTranslation();
  const focusReturn = useFocusReturn();
  const heading = useRef<HTMLHeadingElement>(null);
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="overlay" />
        <DialogPrimitive.Content
          className={`task-dialog task-dialog--${size} ${className}`}
          {...focusReturn}
          onPointerDownOutside={(event) => {
            if (preventOutsideClose) event.preventDefault();
          }}
          onOpenAutoFocus={(event) => {
            focusReturn.onOpenAutoFocus();
            event.preventDefault();
            (initialFocus?.current ?? heading.current)?.focus({
              preventScroll: true,
            });
          }}
        >
          <div className="task-dialog-header">
            <div>
              <DialogPrimitive.Title ref={heading} tabIndex={-1}>
                {title}
              </DialogPrimitive.Title>
              <DialogPrimitive.Description>
                {description}
              </DialogPrimitive.Description>
            </div>
            <DialogPrimitive.Close asChild>
              <Button variant="ghost" size="icon" aria-label={t("close")}>
                <X size={18} />
              </Button>
            </DialogPrimitive.Close>
          </div>
          <div className="task-dialog-body" data-overlay-body>
            {children}
          </div>
          {footer && <div className="task-dialog-footer">{footer}</div>}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

export function Drawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  initialFocus,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  children: ReactNode;
  footer?: ReactNode;
  initialFocus?: RefObject<HTMLInputElement>;
}) {
  const { t } = useTranslation();
  const focusReturn = useFocusReturn();
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="overlay" />
        <DialogPrimitive.Content
          className="drawer"
          {...focusReturn}
          onOpenAutoFocus={(event) => {
            focusReturn.onOpenAutoFocus();
            if (initialFocus?.current) {
              event.preventDefault();
              initialFocus.current.focus();
            }
          }}
        >
          <div className="drawer-header">
            <div>
              <DialogPrimitive.Title>{title}</DialogPrimitive.Title>
              <DialogPrimitive.Description>
                {description}
              </DialogPrimitive.Description>
            </div>
            <DialogPrimitive.Close asChild>
              <Button variant="ghost" size="icon" aria-label={t("close")}>
                <X />
              </Button>
            </DialogPrimitive.Close>
          </div>
          <div className="drawer-body" data-overlay-body>
            {children}
          </div>
          {footer && <div className="drawer-footer">{footer}</div>}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
export function ConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
  title,
  description,
  confirmLabel,
  cancelLabel,
  variant = "danger",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  title?: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "danger" | "primary";
}) {
  const { t } = useTranslation();
  const focusReturn = useFocusReturn();
  return (
    <AlertPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <AlertPrimitive.Portal>
        <AlertPrimitive.Overlay className="overlay" />
        <AlertPrimitive.Content
          className="modal confirm-dialog"
          {...focusReturn}
        >
          <AlertPrimitive.Title>
            {title ?? t("confirmTitle")}
          </AlertPrimitive.Title>
          <AlertPrimitive.Description>
            {description ?? t("confirmDescription")}
          </AlertPrimitive.Description>
          <div className="modal-actions">
            <AlertPrimitive.Cancel asChild>
              <Button>{cancelLabel ?? t("cancel")}</Button>
            </AlertPrimitive.Cancel>
            <AlertPrimitive.Action asChild>
              <Button variant={variant} onClick={onConfirm}>
                {confirmLabel ?? t("confirm")}
              </Button>
            </AlertPrimitive.Action>
          </div>
        </AlertPrimitive.Content>
      </AlertPrimitive.Portal>
    </AlertPrimitive.Root>
  );
}
