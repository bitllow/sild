import * as React from "react";

export interface ComposerBarProps
  extends Omit<React.HTMLAttributes<HTMLDivElement>, "onChange"> {
  value?: string;
  onChange?: (value: string, e: React.ChangeEvent<HTMLTextAreaElement>) => void;
  onSend?: () => void;
  onAttach?: () => void;
  placeholder?: string;
  internal?: boolean;
  onToggleInternal?: (v: boolean) => void;
  showInternalToggle?: boolean;
  disabled?: boolean;
  /** Allow sending with empty text (e.g. when attachments are queued). */
  canSendEmpty?: boolean;
}

const Paperclip = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="m21.44 11.05-9.19 9.19a6 6 0 01-8.49-8.49l8.57-8.57A4 4 0 1118 8.84l-8.59 8.57a2 2 0 01-2.83-2.83l8.49-8.48" />
  </svg>
);

const Send = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M22 2 11 13M22 2l-7 20-4-9-9-4 20-7" />
  </svg>
);

export function ComposerBar({
  value,
  onChange,
  onSend,
  onAttach,
  placeholder,
  internal = false,
  onToggleInternal,
  showInternalToggle = false,
  disabled = false,
  canSendEmpty = false,
  className = "",
  ...rest
}: ComposerBarProps) {
  const ref = React.useRef<HTMLTextAreaElement>(null);
  const [internalState, setInternalState] = React.useState(false);
  const isInternal = onToggleInternal ? internal : internalState;
  const setInternal = onToggleInternal || setInternalState;

  React.useEffect(() => {
    const el = ref.current;
    if (el) {
      el.style.height = "auto";
      el.style.height = Math.min(el.scrollHeight, 140) + "px";
    }
  }, [value]);

  const ph =
    placeholder || (isInternal ? "Add an internal note (only your team sees this)…" : "Write a reply…");

  const canSend = !disabled && (!!value?.trim() || canSendEmpty);
  const handleKey = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (canSend && onSend) onSend();
    }
  };

  return (
    <div
      className={["sild-composer", isInternal ? "sild-composer--internal" : "", className].filter(Boolean).join(" ")}
      {...rest}
    >
      {showInternalToggle && (
        <div className="sild-composer__bar">
          <div className="sild-composer__seg">
            <button
              type="button"
              className={["sild-composer__tab", !isInternal ? "sild-composer__tab--on" : ""].filter(Boolean).join(" ")}
              onClick={() => setInternal(false)}
            >
              Reply
            </button>
            <button
              type="button"
              className={["sild-composer__tab", isInternal ? "sild-composer__tab--on" : ""].filter(Boolean).join(" ")}
              onClick={() => setInternal(true)}
            >
              Internal note
            </button>
          </div>
          <span className="sild-composer__spacer" />
        </div>
      )}
      <div className="sild-composer__row">
        <button type="button" className="sild-composer__icon" aria-label="Attach file" onClick={onAttach}>
          <Paperclip />
        </button>
        <textarea
          ref={ref}
          className="sild-composer__input"
          rows={1}
          placeholder={ph}
          value={value}
          disabled={disabled}
          onChange={(e) => onChange && onChange(e.target.value, e)}
          onKeyDown={handleKey}
        />
        <button
          type="button"
          className="sild-composer__icon sild-composer__send"
          aria-label="Send"
          disabled={!canSend}
          onClick={onSend}
        >
          <Send />
        </button>
      </div>
    </div>
  );
}
