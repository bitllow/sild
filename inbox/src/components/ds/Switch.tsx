import * as React from "react";

export interface SwitchProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "onChange"> {
  checked?: boolean;
  onChange?: (checked: boolean, e: React.ChangeEvent<HTMLInputElement>) => void;
  label?: React.ReactNode;
}

export function Switch({ checked = false, onChange, label, disabled = false, className = "", ...rest }: SwitchProps) {
  return (
    <label className={["sild-switch", className].filter(Boolean).join(" ")} aria-disabled={disabled}>
      <input
        type="checkbox"
        role="switch"
        className="sild-switch__hide"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange && onChange(e.target.checked, e)}
        {...rest}
      />
      <span className={["sild-switch__track", checked ? "sild-switch__track--on" : ""].filter(Boolean).join(" ")}>
        <span className="sild-switch__knob" />
      </span>
      {label && <span className="sild-switch__txt">{label}</span>}
    </label>
  );
}
