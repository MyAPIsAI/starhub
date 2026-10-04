import { forwardRef, type InputHTMLAttributes, type ReactNode } from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import * as SwitchPrimitive from "@radix-ui/react-switch";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import {
  Check,
  ChevronDown,
  CircleCheck,
  CircleAlert,
  CircleX,
  Info,
} from "lucide-react";
import { cn } from "../../lib/utils";
import { usePreferences } from "../../lib/preferences";

export const Input = forwardRef<
  HTMLInputElement,
  InputHTMLAttributes<HTMLInputElement>
>(function Input({ className, ...props }, ref) {
  return <input ref={ref} className={cn("input", className)} {...props} />;
});
export function Field({
  label,
  id,
  hint,
  error,
  children,
}: {
  label: string;
  id: string;
  hint?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {children}
      {(error || hint) && (
        <p
          id={`${id}-help`}
          className={error ? "field-error" : "field-hint"}
          role={error ? "alert" : undefined}
        >
          {error || hint}
        </p>
      )}
    </div>
  );
}
export function Select({
  value,
  onChange,
  options,
  label,
  id,
}: {
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
  label: string;
  id?: string;
}) {
  const rtl = usePreferences((s) => s.language === "ar");
  return (
    <SelectPrimitive.Root
      value={value}
      onValueChange={onChange}
      dir={rtl ? "rtl" : "ltr"}
    >
      <SelectPrimitive.Trigger
        className="select-trigger"
        id={id}
        aria-label={label}
      >
        <SelectPrimitive.Value />
        <SelectPrimitive.Icon className="select-trigger-icon">
          <ChevronDown size={14} />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          className="select-content"
          position="popper"
          sideOffset={6}
        >
          <SelectPrimitive.Viewport>
            {options.map((option) => (
              <SelectPrimitive.Item
                className="select-item"
                value={option.value}
                key={option.value}
              >
                <SelectPrimitive.ItemText>
                  {option.label}
                </SelectPrimitive.ItemText>
                <SelectPrimitive.ItemIndicator>
                  <Check size={14} />
                </SelectPrimitive.ItemIndicator>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}
export function Switch({
  checked,
  onChange,
  id,
  label,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  id: string;
  label: string;
}) {
  return (
    <SwitchPrimitive.Root
      type="button"
      checked={checked}
      onCheckedChange={onChange}
      id={id}
      aria-label={label}
      className="switch"
    >
      <SwitchPrimitive.Thumb className="switch-thumb" />
    </SwitchPrimitive.Root>
  );
}
export function Segmented({
  value,
  onChange,
  options,
  label,
}: {
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
  label: string;
}) {
  return (
    <div className="segmented" role="group" aria-label={label}>
      {options.map((item) => (
        <button
          type="button"
          key={item.value}
          aria-pressed={value === item.value}
          onClick={() => onChange(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
export function Badge({
  children,
  tone = "neutral",
  dot = false,
}: {
  children: ReactNode;
  tone?: "neutral" | "success" | "warning" | "danger" | "info";
  dot?: boolean;
}) {
  return (
    <span className={cn("badge", `badge-${tone}`)}>
      {dot && <span className="status-dot" aria-hidden="true" />}
      {children}
    </span>
  );
}
export function Status({
  label,
  tone,
}: {
  label: string;
  tone: "success" | "warning" | "danger" | "info";
}) {
  const Icon = {
    success: CircleCheck,
    warning: CircleAlert,
    danger: CircleX,
    info: Info,
  }[tone];
  return (
    <span className={cn("status", `status-${tone}`)}>
      <Icon size={14} aria-hidden="true" />
      {label}
    </span>
  );
}
export function Tooltip({
  children,
  text,
}: {
  children: ReactNode;
  text: string;
}) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content className="tooltip" sideOffset={6}>
          {text}
          <TooltipPrimitive.Arrow className="tooltip-arrow" />
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
export function Progress({ value, label }: { value: number; label: string }) {
  return (
    <div
      className="progress-track"
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={value}
    >
      <span
        className={
          value >= 100
            ? "progress-fill danger-fill"
            : value >= 90
              ? "progress-fill warning-fill"
              : "progress-fill"
        }
        style={{ inlineSize: `${Math.min(100, Math.max(0, value))}%` }}
      />
    </div>
  );
}
