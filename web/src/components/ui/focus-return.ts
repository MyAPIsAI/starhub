import { useRef } from "react";

/** Controlled dialogs have no Radix Trigger; remember their actual opener. */
export function useFocusReturn() {
  const opener = useRef<HTMLElement | null>(null);
  return {
    onOpenAutoFocus: () => {
      opener.current =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;
    },
    onCloseAutoFocus: (event: Event) => {
      event.preventDefault();
      const target = opener.current?.isConnected
        ? opener.current
        : document.querySelector<HTMLElement>("main h1, main, [role='main']");
      if (!target) return;
      if (!target.hasAttribute("tabindex") && target.tabIndex < 0) {
        target.setAttribute("tabindex", "-1");
        target.addEventListener(
          "blur",
          () => target.removeAttribute("tabindex"),
          { once: true },
        );
      }
      target.focus({ preventScroll: true });
    },
  };
}
