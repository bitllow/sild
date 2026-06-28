import * as React from "react";

export interface DialogProps {
  open?: boolean;
  onClose?: () => void;
  title?: React.ReactNode;
  subtitle?: React.ReactNode;
  footer?: React.ReactNode;
  width?: number | string;
  className?: string;
  children?: React.ReactNode;
}

export function Dialog({
  open = true,
  onClose,
  title,
  subtitle,
  footer,
  width = 480,
  className = "",
  children,
}: DialogProps) {
  if (!open) return null;
  return (
    <div
      className="sild-dialog__scrim"
      onClick={(e) => {
        if (e.target === e.currentTarget && onClose) onClose();
      }}
    >
      <div
        className={["sild-dialog", className].filter(Boolean).join(" ")}
        role="dialog"
        aria-modal="true"
        style={{ "--_w": typeof width === "number" ? `${width}px` : width } as React.CSSProperties}
      >
        {(title || onClose) && (
          <div className="sild-dialog__head">
            <div>
              {title && <div className="sild-dialog__title">{title}</div>}
              {subtitle && <div className="sild-dialog__sub">{subtitle}</div>}
            </div>
            {onClose && (
              <button type="button" className="sild-dialog__x" aria-label="Close" onClick={onClose}>
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                  <path d="M18 6 6 18M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>
        )}
        <div className="sild-dialog__body">{children}</div>
        {footer && <div className="sild-dialog__foot">{footer}</div>}
      </div>
    </div>
  );
}
