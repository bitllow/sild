import * as React from "react";

export type SelectOption = string | { value: string; label: string };

export interface SelectProps extends Omit<React.SelectHTMLAttributes<HTMLSelectElement>, "size"> {
  label?: string;
  hint?: string;
  error?: string;
  size?: "sm" | "md" | "lg";
  options?: SelectOption[];
  placeholder?: string;
}

export function Select({
  label,
  hint,
  error,
  required = false,
  size = "md",
  options = [],
  placeholder,
  className = "",
  id,
  children,
  ...rest
}: SelectProps) {
  const fid = id || (label ? "sel-" + label.replace(/\s+/g, "-").toLowerCase() : undefined);
  return (
    <div className={["sild-field", className].filter(Boolean).join(" ")}>
      {label && (
        <label className="sild-field__label" htmlFor={fid}>
          {label}
          {required && <span className="sild-field__req">*</span>}
        </label>
      )}
      <div
        className={["sild-select", `sild-select--${size}`].filter(Boolean).join(" ")}
        style={error ? { borderColor: "var(--danger)" } : undefined}
      >
        <select id={fid} className="sild-select__el" {...rest}>
          {placeholder && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {options.map((o) => {
            const val = typeof o === "string" ? o : o.value;
            const lab = typeof o === "string" ? o : o.label;
            return (
              <option key={val} value={val}>
                {lab}
              </option>
            );
          })}
          {children}
        </select>
        <span className="sild-select__chev">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </span>
      </div>
      {(error || hint) && (
        <span className={["sild-field__hint", error ? "sild-field__hint--error" : ""].filter(Boolean).join(" ")}>
          {error || hint}
        </span>
      )}
    </div>
  );
}
