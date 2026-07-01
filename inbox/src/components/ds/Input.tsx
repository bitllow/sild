import * as React from "react";

export interface InputProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "size"> {
  label?: string;
  hint?: string;
  error?: string;
  size?: "sm" | "md" | "lg";
  iconLeft?: React.ReactNode;
}

export function Input({
  label,
  hint,
  error,
  size = "md",
  iconLeft,
  className = "",
  id,
  ...rest
}: InputProps) {
  const fid = id || (label ? "in-" + label.replace(/\s+/g, "-").toLowerCase() : undefined);
  return (
    <div className={["sild-field", className].filter(Boolean).join(" ")}>
      {label && (
        <label className="sild-field__label" htmlFor={fid}>
          {label}
        </label>
      )}
      <div className={["sild-input", `sild-input--${size}`, error ? "sild-input--error" : ""].filter(Boolean).join(" ")}>
        {iconLeft && <span className="sild-input__icon">{iconLeft}</span>}
        <input id={fid} className="sild-input__el" {...rest} />
      </div>
      {(error || hint) && (
        <span className={["sild-field__hint", error ? "sild-field__hint--error" : ""].filter(Boolean).join(" ")}>
          {error || hint}
        </span>
      )}
    </div>
  );
}
