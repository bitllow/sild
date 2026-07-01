import * as React from "react";

export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  hint?: string;
  error?: string;
}

export function Textarea({ label, hint, error, className = "", id, ...rest }: TextareaProps) {
  const fid = id || (label ? "ta-" + label.replace(/\s+/g, "-").toLowerCase() : undefined);
  return (
    <div className={["sild-field", className].filter(Boolean).join(" ")}>
      {label && (
        <label className="sild-field__label" htmlFor={fid}>
          {label}
        </label>
      )}
      <textarea
        id={fid}
        className={["sild-textarea", error ? "sild-textarea--error" : ""].filter(Boolean).join(" ")}
        {...rest}
      />
      {(error || hint) && (
        <span className={["sild-field__hint", error ? "sild-field__hint--error" : ""].filter(Boolean).join(" ")}>
          {error || hint}
        </span>
      )}
    </div>
  );
}
