import * as React from "react";

export type BannerVariant = "info" | "success" | "warning" | "danger";

const ICONS: Record<BannerVariant, string> = {
  info: "M12 16v-4M12 8h.01M12 22a10 10 0 100-20 10 10 0 000 20z",
  success: "M22 11.08V12a10 10 0 11-5.93-9.14M22 4 12 14.01l-3-3",
  warning: "M10.29 3.86 1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0zM12 9v4M12 17h.01",
  danger: "M12 8v4M12 16h.01M12 22a10 10 0 100-20 10 10 0 000 20z",
};

export interface BannerProps extends Omit<React.HTMLAttributes<HTMLDivElement>, "title"> {
  variant?: BannerVariant;
  title?: React.ReactNode;
  onClose?: () => void;
}

export function Banner({ variant = "info", title, onClose, className = "", children, ...rest }: BannerProps) {
  return (
    <div className={["sild-banner", `sild-banner--${variant}`, className].filter(Boolean).join(" ")} role="status" {...rest}>
      <span className="sild-banner__icon">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d={ICONS[variant]} />
        </svg>
      </span>
      <div className="sild-banner__body">
        {title && <div className="sild-banner__title">{title}</div>}
        {children && <div className="sild-banner__msg">{children}</div>}
      </div>
      {onClose && (
        <button type="button" className="sild-banner__close" aria-label="Dismiss" onClick={onClose}>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            <path d="M18 6 6 18M6 6l12 12" />
          </svg>
        </button>
      )}
    </div>
  );
}
