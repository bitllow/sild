import * as React from "react";

export interface TagProps extends React.HTMLAttributes<HTMLSpanElement> {
  mono?: boolean;
  onRemove?: () => void;
}

export function Tag({ mono = false, onRemove, className = "", children, ...rest }: TagProps) {
  const cls = ["sild-tag", mono ? "sild-tag--mono" : "", className].filter(Boolean).join(" ");
  return (
    <span className={cls} {...rest}>
      {children}
      {onRemove && (
        <button type="button" className="sild-tag__remove" aria-label="Remove" onClick={onRemove}>
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
            <path d="M3 3l6 6M9 3l-6 6" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
          </svg>
        </button>
      )}
    </span>
  );
}
