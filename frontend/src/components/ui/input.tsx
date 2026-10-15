"use client";
import { forwardRef, type InputHTMLAttributes, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

const base =
  "w-full h-8 rounded-md border border-border bg-bg px-2.5 text-[13px] text-fg placeholder:text-fg-subtle focus:border-accent disabled:opacity-50";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className, ...rest },
  ref,
) {
  return <input ref={ref} className={cn(base, className)} {...rest} />;
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function Textarea({ className, ...rest }, ref) {
    return <textarea ref={ref} className={cn(base, "h-auto min-h-16 py-1.5", className)} {...rest} />;
  },
);

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  options: SelectOption[];
  placeholder?: string;
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { className, options, placeholder, ...rest },
  ref,
) {
  return (
    <select ref={ref} className={cn(base, "pr-7", className)} {...rest}>
      {placeholder !== undefined && <option value="">{placeholder}</option>}
      {options.map((o) => (
        <option key={o.value} value={o.value} disabled={o.disabled}>
          {o.label}
        </option>
      ))}
    </select>
  );
});

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {
  label: string;
}

export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(function Checkbox(
  { label, className, ...rest },
  ref,
) {
  return (
    <label className={cn("inline-flex items-center gap-2 text-[13px] cursor-pointer select-none", className)}>
      <input ref={ref} type="checkbox" className="size-4" {...rest} />
      <span>{label}</span>
    </label>
  );
});

export interface SliderProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {
  valueLabel?: string;
}

export const Slider = forwardRef<HTMLInputElement, SliderProps>(function Slider({ valueLabel, className, ...rest }, ref) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <input ref={ref} type="range" className="flex-1" {...rest} />
      {valueLabel !== undefined && <span className="tabular text-xs w-12 text-right text-fg-muted">{valueLabel}</span>}
    </div>
  );
});

export function Field({
  label,
  htmlFor,
  hint,
  error,
  className,
  children,
  required,
}: {
  label: string;
  htmlFor?: string;
  hint?: string;
  error?: string;
  className?: string;
  children: React.ReactNode;
  required?: boolean;
}) {
  return (
    <div className={cn("flex flex-col gap-1", className)}>
      <label htmlFor={htmlFor} className="text-xs font-medium text-fg-muted">
        {label}
        {required && <span className="text-danger ml-0.5" aria-hidden>*</span>}
      </label>
      {children}
      {hint && !error && <p className="text-[11px] text-fg-subtle">{hint}</p>}
      {error && (
        <p className="text-[11px] text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
