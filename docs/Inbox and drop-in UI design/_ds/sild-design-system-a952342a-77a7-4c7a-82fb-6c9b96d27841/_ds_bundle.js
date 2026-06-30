/* @ds-bundle: {"format":3,"namespace":"SildDesignSystem_a95234","components":[{"name":"ComposerBar","sourcePath":"components/chat/ComposerBar.jsx"},{"name":"ConversationRow","sourcePath":"components/chat/ConversationRow.jsx"},{"name":"MessageBubble","sourcePath":"components/chat/MessageBubble.jsx"},{"name":"StatusPill","sourcePath":"components/chat/StatusPill.jsx"},{"name":"Avatar","sourcePath":"components/core/Avatar.jsx"},{"name":"AvatarStack","sourcePath":"components/core/AvatarStack.jsx"},{"name":"Badge","sourcePath":"components/core/Badge.jsx"},{"name":"Button","sourcePath":"components/core/Button.jsx"},{"name":"IconButton","sourcePath":"components/core/IconButton.jsx"},{"name":"Spinner","sourcePath":"components/core/Spinner.jsx"},{"name":"Tag","sourcePath":"components/core/Tag.jsx"},{"name":"Banner","sourcePath":"components/feedback/Banner.jsx"},{"name":"Card","sourcePath":"components/feedback/Card.jsx"},{"name":"Dialog","sourcePath":"components/feedback/Dialog.jsx"},{"name":"Tooltip","sourcePath":"components/feedback/Tooltip.jsx"},{"name":"Checkbox","sourcePath":"components/forms/Checkbox.jsx"},{"name":"Input","sourcePath":"components/forms/Input.jsx"},{"name":"Select","sourcePath":"components/forms/Select.jsx"},{"name":"Switch","sourcePath":"components/forms/Switch.jsx"},{"name":"Textarea","sourcePath":"components/forms/Textarea.jsx"}],"sourceHashes":{"components/chat/ComposerBar.jsx":"2f2cabc13884","components/chat/ConversationRow.jsx":"0b2b8583d834","components/chat/MessageBubble.jsx":"49559570be73","components/chat/StatusPill.jsx":"27a45d0487f6","components/core/Avatar.jsx":"c5ff1dec890b","components/core/AvatarStack.jsx":"56c7b030b77e","components/core/Badge.jsx":"99532ec53349","components/core/Button.jsx":"a3d5a8e14580","components/core/IconButton.jsx":"6621a9de8923","components/core/Spinner.jsx":"30c44b654fc3","components/core/Tag.jsx":"21d842eed667","components/feedback/Banner.jsx":"2ed02690d61a","components/feedback/Card.jsx":"1c93e4dca4c8","components/feedback/Dialog.jsx":"7ab1993e91c3","components/feedback/Tooltip.jsx":"1f59713e1b2c","components/forms/Checkbox.jsx":"ee45257914d2","components/forms/Input.jsx":"d301ed3df673","components/forms/Select.jsx":"b5aae213e6c8","components/forms/Switch.jsx":"120a5c9b75c2","components/forms/Textarea.jsx":"4de326b247cb","ui_kits/icons.jsx":"c619d37eb3f2","ui_kits/inbox/App.jsx":"6c92c947cac9","ui_kits/inbox/Inbox.jsx":"df6bfd8ab23f","ui_kits/inbox/Shell.jsx":"960ddbfb48bd","ui_kits/inbox/data.js":"c12763bd78d0","ui_kits/sild-bundle.jsx":"b4b147dca35f","ui_kits/widget/Widget.jsx":"424cbd09fdcb"},"inlinedExternals":[],"unexposedExports":[]} */

(() => {

const __ds_ns = (window.SildDesignSystem_a95234 = window.SildDesignSystem_a95234 || {});

const __ds_scope = {};

(__ds_ns.__errors = __ds_ns.__errors || []);

// components/chat/ComposerBar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-composer{font-family:var(--font-sans);background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-lg);transition:border-color var(--duration-fast),box-shadow var(--duration-fast)}
.sild-composer:focus-within{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-composer--internal{background:var(--warning-subtle);border-color:var(--amber-500)}
.sild-composer--internal:focus-within{box-shadow:0 0 0 3px rgba(245,165,36,.28)}
.sild-composer__bar{display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid var(--border-subtle)}
.sild-composer--internal .sild-composer__bar{border-bottom-color:rgba(245,165,36,.35)}
.sild-composer__seg{display:inline-flex;background:var(--surface-sunken);border-radius:var(--radius-md);padding:2px;gap:2px}
.sild-composer__tab{border:0;background:transparent;font-family:inherit;font-size:12px;font-weight:600;
  color:var(--text-secondary);padding:4px 10px;border-radius:var(--radius-sm);cursor:pointer}
.sild-composer__tab--on{background:var(--white);color:var(--text-primary);box-shadow:var(--shadow-xs)}
.sild-composer--internal .sild-composer__tab--on{background:var(--amber-500);color:#fff}
.sild-composer__spacer{flex:1}
.sild-composer__row{display:flex;align-items:flex-end;gap:8px;padding:8px 10px}
.sild-composer__input{flex:1;border:0;outline:none;background:transparent;resize:none;font-family:inherit;
  font-size:14px;line-height:1.5;color:var(--text-primary);max-height:140px;padding:6px 2px}
.sild-composer__input::placeholder{color:var(--text-tertiary)}
.sild-composer__icon{display:inline-flex;align-items:center;justify-content:center;width:34px;height:34px;border-radius:var(--radius-md);
  border:0;background:transparent;color:var(--text-tertiary);cursor:pointer;flex:none;transition:background var(--duration-fast),color var(--duration-fast)}
.sild-composer__icon:hover{background:var(--surface-hover);color:var(--text-primary)}
.sild-composer__send{background:var(--brand);color:#fff}
.sild-composer__send:hover{background:var(--brand-hover);color:#fff}
.sild-composer__send:disabled{opacity:.4;cursor:not-allowed;background:var(--brand)}
.sild-composer--internal .sild-composer__send{background:var(--amber-500)}
.sild-composer--internal .sild-composer__send:hover{background:var(--amber-600)}
`;
  document.head.appendChild(s);
}
const Paperclip = () => /*#__PURE__*/React.createElement("svg", {
  width: "18",
  height: "18",
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: "2",
  strokeLinecap: "round",
  strokeLinejoin: "round"
}, /*#__PURE__*/React.createElement("path", {
  d: "m21.44 11.05-9.19 9.19a6 6 0 01-8.49-8.49l8.57-8.57A4 4 0 1118 8.84l-8.59 8.57a2 2 0 01-2.83-2.83l8.49-8.48"
}));
const Send = () => /*#__PURE__*/React.createElement("svg", {
  width: "18",
  height: "18",
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: "2",
  strokeLinecap: "round",
  strokeLinejoin: "round"
}, /*#__PURE__*/React.createElement("path", {
  d: "M22 2 11 13M22 2l-7 20-4-9-9-4 20-7"
}));
function ComposerBar({
  value,
  onChange,
  onSend,
  onAttach,
  placeholder,
  internal = false,
  onToggleInternal,
  showInternalToggle = false,
  disabled = false,
  className = '',
  ...rest
}) {
  injectCss();
  const ref = React.useRef(null);
  const [internalState, setInternalState] = React.useState(false);
  const isInternal = onToggleInternal ? internal : internalState;
  const setInternal = onToggleInternal || setInternalState;
  React.useEffect(() => {
    const el = ref.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 140) + 'px';
    }
  }, [value]);
  const ph = placeholder || (isInternal ? 'Add an internal note (only your team sees this)…' : 'Write a reply…');
  const handleKey = e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (value && value.trim() && onSend) onSend();
    }
  };
  return /*#__PURE__*/React.createElement("div", _extends({
    className: ['sild-composer', isInternal ? 'sild-composer--internal' : '', className].filter(Boolean).join(' ')
  }, rest), showInternalToggle && /*#__PURE__*/React.createElement("div", {
    className: "sild-composer__bar"
  }, /*#__PURE__*/React.createElement("div", {
    className: "sild-composer__seg"
  }, /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: ['sild-composer__tab', !isInternal ? 'sild-composer__tab--on' : ''].filter(Boolean).join(' '),
    onClick: () => setInternal(false)
  }, "Reply"), /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: ['sild-composer__tab', isInternal ? 'sild-composer__tab--on' : ''].filter(Boolean).join(' '),
    onClick: () => setInternal(true)
  }, "Internal note")), /*#__PURE__*/React.createElement("span", {
    className: "sild-composer__spacer"
  })), /*#__PURE__*/React.createElement("div", {
    className: "sild-composer__row"
  }, /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: "sild-composer__icon",
    "aria-label": "Attach file",
    onClick: onAttach
  }, /*#__PURE__*/React.createElement(Paperclip, null)), /*#__PURE__*/React.createElement("textarea", {
    ref: ref,
    className: "sild-composer__input",
    rows: 1,
    placeholder: ph,
    value: value,
    disabled: disabled,
    onChange: e => onChange && onChange(e.target.value, e),
    onKeyDown: handleKey
  }), /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: "sild-composer__icon sild-composer__send",
    "aria-label": "Send",
    disabled: disabled || !value || !value.trim(),
    onClick: onSend
  }, /*#__PURE__*/React.createElement(Send, null))));
}
Object.assign(__ds_scope, { ComposerBar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/chat/ComposerBar.jsx", error: String((e && e.message) || e) }); }

// components/chat/MessageBubble.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-msg{display:flex;flex-direction:column;max-width:74%;font-family:var(--font-sans)}
.sild-msg--in{align-self:flex-start;align-items:flex-start}
.sild-msg--out{align-self:flex-end;align-items:flex-end}
.sild-msg--system{align-self:center;align-items:center;max-width:90%}
.sild-msg__meta{display:flex;align-items:center;gap:7px;margin-bottom:4px;padding:0 4px}
.sild-msg__author{font-size:12px;font-weight:600;color:var(--text-secondary)}
.sild-msg__time{font-size:11px;color:var(--text-tertiary)}
.sild-msg__bubble{font-size:14px;line-height:1.5;padding:9px 13px;border-radius:var(--radius-bubble);
  word-break:break-word;white-space:pre-wrap}
.sild-msg--in .sild-msg__bubble{background:var(--surface-sunken);color:var(--text-primary);border-bottom-left-radius:var(--radius-xs)}
.sild-msg--out .sild-msg__bubble{background:var(--brand);color:#fff;border-bottom-right-radius:var(--radius-xs)}
.sild-msg--internal .sild-msg__bubble{background:var(--warning-subtle);color:var(--slate-800);
  border:1px dashed var(--amber-500);border-radius:var(--radius-md)}
.sild-msg--system .sild-msg__bubble{background:transparent;color:var(--text-tertiary);font-size:12px;padding:4px 8px}
.sild-msg__chan{display:inline-flex;align-items:center;gap:4px;font-size:11px;font-weight:600;color:var(--blue-700);
  background:var(--blue-50);border-radius:var(--radius-full);padding:1px 7px}
.sild-msg__intlabel{display:inline-flex;align-items:center;gap:4px;font-size:11px;font-weight:600;color:var(--amber-600)}
.sild-msg__atts{display:flex;flex-direction:column;gap:6px;margin-top:6px}
.sild-msg__att{display:flex;align-items:center;gap:8px;background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-md);padding:7px 10px;font-size:13px;color:var(--text-primary)}
.sild-msg__att-img{margin-top:6px;border-radius:var(--radius-md);max-width:240px;border:1px solid var(--border-subtle)}
.sild-msg__read{font-size:11px;color:var(--text-tertiary);margin-top:3px;padding:0 4px}
`;
  document.head.appendChild(s);
}
const Paperclip = () => /*#__PURE__*/React.createElement("svg", {
  width: "14",
  height: "14",
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: "2",
  strokeLinecap: "round",
  strokeLinejoin: "round"
}, /*#__PURE__*/React.createElement("path", {
  d: "m21.44 11.05-9.19 9.19a6 6 0 01-8.49-8.49l8.57-8.57A4 4 0 1118 8.84l-8.59 8.57a2 2 0 01-2.83-2.83l8.49-8.48"
}));
const Mail = () => /*#__PURE__*/React.createElement("svg", {
  width: "12",
  height: "12",
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: "2",
  strokeLinecap: "round",
  strokeLinejoin: "round"
}, /*#__PURE__*/React.createElement("rect", {
  x: "2",
  y: "4",
  width: "20",
  height: "16",
  rx: "2"
}), /*#__PURE__*/React.createElement("path", {
  d: "m22 7-10 5L2 7"
}));
const LockGlyph = () => /*#__PURE__*/React.createElement("svg", {
  width: "11",
  height: "11",
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: "2.2",
  strokeLinecap: "round",
  strokeLinejoin: "round"
}, /*#__PURE__*/React.createElement("rect", {
  x: "3",
  y: "11",
  width: "18",
  height: "11",
  rx: "2"
}), /*#__PURE__*/React.createElement("path", {
  d: "M7 11V7a5 5 0 0110 0v4"
}));
function MessageBubble({
  direction = 'in',
  author,
  time,
  body,
  channel,
  internal = false,
  system = false,
  attachments = [],
  readReceipt,
  className = '',
  ...rest
}) {
  injectCss();
  const kind = system ? 'system' : direction;
  const cls = ['sild-msg', `sild-msg--${kind}`, internal ? 'sild-msg--internal' : '', className].filter(Boolean).join(' ');
  if (system) {
    return /*#__PURE__*/React.createElement("div", _extends({
      className: cls
    }, rest), /*#__PURE__*/React.createElement("div", {
      className: "sild-msg__bubble"
    }, body));
  }
  return /*#__PURE__*/React.createElement("div", _extends({
    className: cls
  }, rest), (author || time || internal || channel === 'email') && /*#__PURE__*/React.createElement("div", {
    className: "sild-msg__meta"
  }, author && /*#__PURE__*/React.createElement("span", {
    className: "sild-msg__author"
  }, author), internal && /*#__PURE__*/React.createElement("span", {
    className: "sild-msg__intlabel"
  }, /*#__PURE__*/React.createElement(LockGlyph, null), " Internal note"), channel === 'email' && /*#__PURE__*/React.createElement("span", {
    className: "sild-msg__chan"
  }, /*#__PURE__*/React.createElement(Mail, null), " Email"), time && /*#__PURE__*/React.createElement("span", {
    className: "sild-msg__time"
  }, time)), /*#__PURE__*/React.createElement("div", {
    className: "sild-msg__bubble"
  }, body, attachments.filter(a => a.disposition === 'inline' && a.kind === 'image').map((a, i) => /*#__PURE__*/React.createElement("img", {
    key: i,
    className: "sild-msg__att-img",
    src: a.url,
    alt: a.filename || ''
  }))), attachments.filter(a => a.disposition !== 'inline' || a.kind !== 'image').length > 0 && /*#__PURE__*/React.createElement("div", {
    className: "sild-msg__atts"
  }, attachments.filter(a => a.disposition !== 'inline' || a.kind !== 'image').map((a, i) => /*#__PURE__*/React.createElement("span", {
    key: i,
    className: "sild-msg__att"
  }, /*#__PURE__*/React.createElement(Paperclip, null), " ", a.filename || 'attachment'))), readReceipt && /*#__PURE__*/React.createElement("div", {
    className: "sild-msg__read"
  }, readReceipt));
}
Object.assign(__ds_scope, { MessageBubble });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/chat/MessageBubble.jsx", error: String((e && e.message) || e) }); }

// components/chat/StatusPill.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-status{display:inline-flex;align-items:center;gap:6px;font-family:var(--font-sans);font-weight:600;
  font-size:12px;line-height:1;border-radius:var(--radius-full);padding:4px 9px 4px 8px;white-space:nowrap;
  background:var(--_bg);color:var(--_fg)}
.sild-status__dot{width:7px;height:7px;border-radius:50%;background:currentColor}
.sild-status--open{--_bg:var(--success-subtle);--_fg:var(--green-600)}
.sild-status--queued{--_bg:var(--warning-subtle);--_fg:var(--amber-600)}
.sild-status--assigned{--_bg:var(--blue-50);--_fg:var(--blue-700)}
.sild-status--closed{--_bg:var(--slate-100);--_fg:var(--slate-600)}
`;
  document.head.appendChild(s);
}
const LABELS = {
  open: 'Open',
  queued: 'Queued',
  assigned: 'Assigned',
  closed: 'Closed'
};
function StatusPill({
  status = 'open',
  label,
  className = '',
  ...rest
}) {
  injectCss();
  return /*#__PURE__*/React.createElement("span", _extends({
    className: ['sild-status', `sild-status--${status}`, className].filter(Boolean).join(' ')
  }, rest), /*#__PURE__*/React.createElement("span", {
    className: "sild-status__dot",
    "aria-hidden": "true"
  }), label || LABELS[status] || status);
}
Object.assign(__ds_scope, { StatusPill });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/chat/StatusPill.jsx", error: String((e && e.message) || e) }); }

// components/core/Avatar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-avatar{position:relative;display:inline-flex;align-items:center;justify-content:center;
  border-radius:var(--radius-full);font-family:var(--font-sans);font-weight:600;color:#fff;
  background:var(--brand);overflow:visible;flex:none;user-select:none}
.sild-avatar--square{border-radius:var(--radius-md)}
.sild-avatar__img{width:100%;height:100%;object-fit:cover;border-radius:inherit}
.sild-avatar__presence{position:absolute;right:-1px;bottom:-1px;border-radius:50%;
  border:2px solid var(--surface-card);box-sizing:content-box}
.sild-avatar__presence--online{background:var(--status-online)}
.sild-avatar__presence--away{background:var(--status-queued)}
.sild-avatar__presence--offline{background:var(--slate-400)}
`;
  document.head.appendChild(s);
}
const SIZES = {
  xs: 22,
  sm: 28,
  md: 36,
  lg: 44,
  xl: 56
};
const PALETTE = ['#3D63FF', '#FF7A45', '#18A957', '#7C5CFF', '#0EA5A5', '#E0599B', '#D9881A', '#2440B8'];
function initials(name = '') {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (!parts.length) return '?';
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}
function colorFor(name = '') {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = h * 31 + name.charCodeAt(i) >>> 0;
  return PALETTE[h % PALETTE.length];
}
function Avatar({
  name = '',
  src = null,
  size = 'md',
  shape = 'circle',
  presence = null,
  className = '',
  style = {},
  ...rest
}) {
  injectCss();
  const px = typeof size === 'number' ? size : SIZES[size] || 36;
  const cls = ['sild-avatar', shape === 'square' ? 'sild-avatar--square' : '', className].filter(Boolean).join(' ');
  const dot = Math.max(8, Math.round(px * 0.28));
  return /*#__PURE__*/React.createElement("span", _extends({
    className: cls,
    style: {
      width: px,
      height: px,
      fontSize: Math.round(px * 0.38),
      background: src ? 'var(--surface-sunken)' : colorFor(name),
      ...style
    },
    title: name || undefined
  }, rest), src ? /*#__PURE__*/React.createElement("img", {
    className: "sild-avatar__img",
    src: src,
    alt: name
  }) : initials(name), presence && /*#__PURE__*/React.createElement("span", {
    className: `sild-avatar__presence sild-avatar__presence--${presence}`,
    style: {
      width: dot,
      height: dot
    }
  }));
}
Object.assign(__ds_scope, { Avatar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Avatar.jsx", error: String((e && e.message) || e) }); }

// components/chat/ConversationRow.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-convrow{display:flex;gap:11px;align-items:flex-start;padding:11px 14px;cursor:pointer;font-family:var(--font-sans);
  border-left:2px solid transparent;transition:background var(--duration-fast)}
.sild-convrow:hover{background:var(--surface-hover)}
.sild-convrow--active{background:var(--surface-selected);border-left-color:var(--brand)}
.sild-convrow--active:hover{background:var(--surface-selected)}
.sild-convrow__main{flex:1;min-width:0}
.sild-convrow__top{display:flex;align-items:baseline;justify-content:space-between;gap:8px}
.sild-convrow__name{font-size:14px;font-weight:600;color:var(--text-primary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.sild-convrow__name--unread{font-weight:700}
.sild-convrow__time{font-size:11px;color:var(--text-tertiary);flex:none}
.sild-convrow__preview{font-size:13px;color:var(--text-secondary);margin-top:2px;display:-webkit-box;-webkit-line-clamp:1;-webkit-box-orient:vertical;overflow:hidden}
.sild-convrow__preview--unread{color:var(--text-primary)}
.sild-convrow__sub{display:flex;align-items:center;gap:7px;margin-top:6px}
.sild-convrow__ref{font-family:var(--font-mono);font-size:11px;color:var(--text-tertiary);background:var(--surface-sunken);padding:1px 6px;border-radius:var(--radius-xs)}
.sild-convrow__chan{display:inline-flex;color:var(--text-tertiary)}
.sild-convrow__right{display:flex;flex-direction:column;align-items:flex-end;gap:6px;flex:none}
.sild-convrow__count{min-width:18px;height:18px;padding:0 5px;border-radius:var(--radius-full);background:var(--accent);
  color:#fff;font-size:11px;font-weight:700;display:inline-flex;align-items:center;justify-content:center}
`;
  document.head.appendChild(s);
}
const Mail = () => /*#__PURE__*/React.createElement("svg", {
  width: "13",
  height: "13",
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: "2",
  strokeLinecap: "round",
  strokeLinejoin: "round"
}, /*#__PURE__*/React.createElement("rect", {
  x: "2",
  y: "4",
  width: "20",
  height: "16",
  rx: "2"
}), /*#__PURE__*/React.createElement("path", {
  d: "m22 7-10 5L2 7"
}));
function ConversationRow({
  name,
  preview,
  time,
  unread = 0,
  active = false,
  channel = 'app',
  reference,
  presence = null,
  src,
  status,
  onClick,
  className = '',
  ...rest
}) {
  injectCss();
  const isUnread = unread > 0;
  return /*#__PURE__*/React.createElement("div", _extends({
    className: ['sild-convrow', active ? 'sild-convrow--active' : '', className].filter(Boolean).join(' '),
    onClick: onClick,
    role: "button",
    tabIndex: 0
  }, rest), /*#__PURE__*/React.createElement(__ds_scope.Avatar, {
    name: name,
    src: src,
    presence: presence,
    size: "md"
  }), /*#__PURE__*/React.createElement("div", {
    className: "sild-convrow__main"
  }, /*#__PURE__*/React.createElement("div", {
    className: "sild-convrow__top"
  }, /*#__PURE__*/React.createElement("span", {
    className: ['sild-convrow__name', isUnread ? 'sild-convrow__name--unread' : ''].filter(Boolean).join(' ')
  }, name), time && /*#__PURE__*/React.createElement("span", {
    className: "sild-convrow__time"
  }, time)), /*#__PURE__*/React.createElement("div", {
    className: ['sild-convrow__preview', isUnread ? 'sild-convrow__preview--unread' : ''].filter(Boolean).join(' ')
  }, preview), (reference || channel === 'email' || status) && /*#__PURE__*/React.createElement("div", {
    className: "sild-convrow__sub"
  }, channel === 'email' && /*#__PURE__*/React.createElement("span", {
    className: "sild-convrow__chan"
  }, /*#__PURE__*/React.createElement(Mail, null)), reference && /*#__PURE__*/React.createElement("span", {
    className: "sild-convrow__ref"
  }, reference), status)), /*#__PURE__*/React.createElement("div", {
    className: "sild-convrow__right"
  }, isUnread && /*#__PURE__*/React.createElement("span", {
    className: "sild-convrow__count"
  }, unread > 99 ? '99+' : unread)));
}
Object.assign(__ds_scope, { ConversationRow });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/chat/ConversationRow.jsx", error: String((e && e.message) || e) }); }

// components/core/AvatarStack.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
function AvatarStack({
  people = [],
  max = 4,
  size = 'sm',
  className = '',
  ...rest
}) {
  const shown = people.slice(0, max);
  const overflow = people.length - shown.length;
  const px = typeof size === 'number' ? size : {
    xs: 22,
    sm: 28,
    md: 36,
    lg: 44,
    xl: 56
  }[size] || 28;
  const overlap = Math.round(px * 0.32);
  return /*#__PURE__*/React.createElement("span", _extends({
    className: className,
    style: {
      display: 'inline-flex',
      alignItems: 'center'
    }
  }, rest), shown.map((p, i) => /*#__PURE__*/React.createElement("span", {
    key: i,
    style: {
      marginLeft: i === 0 ? 0 : -overlap,
      borderRadius: '50%',
      boxShadow: '0 0 0 2px var(--surface-card)'
    }
  }, /*#__PURE__*/React.createElement(__ds_scope.Avatar, {
    name: p.name,
    src: p.src,
    size: size
  }))), overflow > 0 && /*#__PURE__*/React.createElement("span", {
    style: {
      marginLeft: -overlap,
      width: px,
      height: px,
      borderRadius: '50%',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'var(--slate-100)',
      color: 'var(--text-secondary)',
      fontFamily: 'var(--font-sans)',
      fontWeight: 600,
      fontSize: Math.round(px * 0.34),
      boxShadow: '0 0 0 2px var(--surface-card)'
    }
  }, "+", overflow));
}
Object.assign(__ds_scope, { AvatarStack });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/AvatarStack.jsx", error: String((e && e.message) || e) }); }

// components/core/Badge.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-badge{display:inline-flex;align-items:center;gap:5px;font-family:var(--font-sans);font-weight:600;
  font-size:12px;line-height:1;border-radius:var(--radius-full);padding:4px 9px;white-space:nowrap;
  --_bg:var(--surface-sunken);--_fg:var(--text-secondary);background:var(--_bg);color:var(--_fg)}
.sild-badge--neutral{--_bg:var(--slate-100);--_fg:var(--slate-700)}
.sild-badge--brand{--_bg:var(--blue-50);--_fg:var(--blue-700)}
.sild-badge--success{--_bg:var(--success-subtle);--_fg:var(--green-600)}
.sild-badge--warning{--_bg:var(--warning-subtle);--_fg:var(--amber-600)}
.sild-badge--danger{--_bg:var(--danger-subtle);--_fg:var(--red-600)}
.sild-badge--accent{--_bg:var(--accent-subtle);--_fg:var(--coral-600)}
.sild-badge--solid{--_bg:var(--brand);--_fg:#fff}
.sild-badge__dot{width:6px;height:6px;border-radius:50%;background:currentColor}
.sild-badge--count{min-width:18px;height:18px;padding:0 5px;justify-content:center;--_bg:var(--accent);--_fg:#fff}
`;
  document.head.appendChild(s);
}
function Badge({
  variant = 'neutral',
  dot = false,
  count = false,
  className = '',
  children,
  ...rest
}) {
  injectCss();
  const cls = ['sild-badge', `sild-badge--${variant}`, count ? 'sild-badge--count' : '', className].filter(Boolean).join(' ');
  return /*#__PURE__*/React.createElement("span", _extends({
    className: cls
  }, rest), dot && /*#__PURE__*/React.createElement("span", {
    className: "sild-badge__dot",
    "aria-hidden": "true"
  }), children);
}
Object.assign(__ds_scope, { Badge });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Badge.jsx", error: String((e && e.message) || e) }); }

// components/core/Button.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-btn{--_bg:var(--brand);--_bg-h:var(--brand-hover);--_bg-a:var(--brand-active);--_fg:#fff;--_bd:transparent;
  display:inline-flex;align-items:center;justify-content:center;gap:8px;font-family:var(--font-sans);
  font-weight:600;letter-spacing:-.01em;border:1px solid var(--_bd);border-radius:var(--radius-md);
  background:var(--_bg);color:var(--_fg);cursor:pointer;white-space:nowrap;
  transition:background var(--duration-fast) var(--ease-standard),box-shadow var(--duration-fast) var(--ease-standard),transform var(--duration-instant) var(--ease-standard);}
.sild-btn:hover{background:var(--_bg-h)}
.sild-btn:active{background:var(--_bg-a);transform:translateY(1px)}
.sild-btn:focus-visible{outline:none;box-shadow:var(--ring)}
.sild-btn[disabled]{cursor:not-allowed;opacity:.5}
.sild-btn[disabled]:active{transform:none}
.sild-btn--md{height:40px;padding:0 16px;font-size:14px}
.sild-btn--sm{height:32px;padding:0 12px;font-size:13px;border-radius:var(--radius-sm)}
.sild-btn--lg{height:48px;padding:0 22px;font-size:16px}
.sild-btn--full{width:100%}
.sild-btn--secondary{--_bg:var(--white);--_bg-h:var(--surface-hover);--_bg-a:var(--surface-active);--_fg:var(--text-primary);--_bd:var(--border-default)}
.sild-btn--ghost{--_bg:transparent;--_bg-h:var(--surface-hover);--_bg-a:var(--surface-active);--_fg:var(--text-primary);--_bd:transparent}
.sild-btn--danger{--_bg:var(--danger);--_bg-h:var(--danger-hover);--_bg-a:var(--red-600);--_fg:#fff}
.sild-btn--danger:focus-visible{box-shadow:var(--ring-danger)}
.sild-btn__spin{width:15px;height:15px;border-radius:50%;border:2px solid currentColor;border-right-color:transparent;animation:sild-btn-spin .6s linear infinite}
@keyframes sild-btn-spin{to{transform:rotate(360deg)}}
@media (prefers-reduced-motion:reduce){.sild-btn{transition:none}.sild-btn__spin{animation-duration:1.2s}}
`;
  document.head.appendChild(s);
}
function Button({
  variant = 'primary',
  size = 'md',
  disabled = false,
  loading = false,
  fullWidth = false,
  iconLeft = null,
  iconRight = null,
  type = 'button',
  className = '',
  children,
  ...rest
}) {
  injectCss();
  const cls = ['sild-btn', `sild-btn--${size}`, variant !== 'primary' ? `sild-btn--${variant}` : '', fullWidth ? 'sild-btn--full' : '', className].filter(Boolean).join(' ');
  return /*#__PURE__*/React.createElement("button", _extends({
    type: type,
    className: cls,
    disabled: disabled || loading
  }, rest), loading && /*#__PURE__*/React.createElement("span", {
    className: "sild-btn__spin",
    "aria-hidden": "true"
  }), !loading && iconLeft, children && /*#__PURE__*/React.createElement("span", null, children), !loading && iconRight);
}
Object.assign(__ds_scope, { Button });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Button.jsx", error: String((e && e.message) || e) }); }

// components/core/IconButton.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-iconbtn{display:inline-flex;align-items:center;justify-content:center;border-radius:var(--radius-md);
  border:1px solid transparent;background:transparent;color:var(--text-secondary);cursor:pointer;
  transition:background var(--duration-fast) var(--ease-standard),color var(--duration-fast) var(--ease-standard);}
.sild-iconbtn:hover{background:var(--surface-hover);color:var(--text-primary)}
.sild-iconbtn:active{background:var(--surface-active)}
.sild-iconbtn:focus-visible{outline:none;box-shadow:var(--ring)}
.sild-iconbtn[disabled]{cursor:not-allowed;opacity:.45}
.sild-iconbtn--sm{width:30px;height:30px}
.sild-iconbtn--md{width:36px;height:36px}
.sild-iconbtn--lg{width:44px;height:44px}
.sild-iconbtn--solid{background:var(--brand);color:#fff}
.sild-iconbtn--solid:hover{background:var(--brand-hover);color:#fff}
.sild-iconbtn--solid:active{background:var(--brand-active)}
.sild-iconbtn--bordered{border-color:var(--border-default);background:var(--white)}
@media (prefers-reduced-motion:reduce){.sild-iconbtn{transition:none}}
`;
  document.head.appendChild(s);
}
function IconButton({
  size = 'md',
  variant = 'ghost',
  disabled = false,
  className = '',
  'aria-label': ariaLabel,
  children,
  ...rest
}) {
  injectCss();
  const cls = ['sild-iconbtn', `sild-iconbtn--${size}`, variant !== 'ghost' ? `sild-iconbtn--${variant}` : '', className].filter(Boolean).join(' ');
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    className: cls,
    disabled: disabled,
    "aria-label": ariaLabel
  }, rest), children);
}
Object.assign(__ds_scope, { IconButton });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/IconButton.jsx", error: String((e && e.message) || e) }); }

// components/core/Spinner.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-spinner{display:inline-block;border-radius:50%;border-style:solid;border-color:var(--border-strong);
  border-right-color:var(--brand);animation:sild-spin .6s linear infinite}
@keyframes sild-spin{to{transform:rotate(360deg)}}
@media (prefers-reduced-motion:reduce){.sild-spinner{animation-duration:1.2s}}
`;
  document.head.appendChild(s);
}
const SIZES = {
  sm: 16,
  md: 22,
  lg: 32
};
function Spinner({
  size = 'md',
  className = '',
  style = {},
  ...rest
}) {
  injectCss();
  const px = typeof size === 'number' ? size : SIZES[size] || 22;
  const bw = Math.max(2, Math.round(px / 9));
  return /*#__PURE__*/React.createElement("span", _extends({
    className: ['sild-spinner', className].filter(Boolean).join(' '),
    role: "status",
    "aria-label": "Loading",
    style: {
      width: px,
      height: px,
      borderWidth: bw,
      ...style
    }
  }, rest));
}
Object.assign(__ds_scope, { Spinner });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Spinner.jsx", error: String((e && e.message) || e) }); }

// components/core/Tag.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-tag{display:inline-flex;align-items:center;gap:6px;font-family:var(--font-sans);font-size:13px;
  font-weight:500;color:var(--text-secondary);background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-sm);padding:3px 8px;line-height:1.4;white-space:nowrap}
.sild-tag--mono{font-family:var(--font-mono);font-size:12px;background:var(--surface-sunken);border-color:var(--border-subtle)}
.sild-tag__remove{display:inline-flex;cursor:pointer;color:var(--text-tertiary);border:0;background:none;padding:0;
  border-radius:var(--radius-xs);transition:color var(--duration-fast)}
.sild-tag__remove:hover{color:var(--text-primary)}
`;
  document.head.appendChild(s);
}
function Tag({
  mono = false,
  onRemove,
  className = '',
  children,
  ...rest
}) {
  injectCss();
  const cls = ['sild-tag', mono ? 'sild-tag--mono' : '', className].filter(Boolean).join(' ');
  return /*#__PURE__*/React.createElement("span", _extends({
    className: cls
  }, rest), children, onRemove && /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: "sild-tag__remove",
    "aria-label": "Remove",
    onClick: onRemove
  }, /*#__PURE__*/React.createElement("svg", {
    width: "12",
    height: "12",
    viewBox: "0 0 12 12",
    fill: "none"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M3 3l6 6M9 3l-6 6",
    stroke: "currentColor",
    strokeWidth: "1.6",
    strokeLinecap: "round"
  }))));
}
Object.assign(__ds_scope, { Tag });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Tag.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Banner.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-banner{display:flex;gap:11px;align-items:flex-start;font-family:var(--font-sans);
  border:1px solid var(--_bd,var(--border-default));background:var(--_bg,var(--surface-sunken));
  border-radius:var(--radius-md);padding:12px 14px;color:var(--text-primary)}
.sild-banner--info{--_bg:var(--blue-50);--_bd:var(--blue-200)}
.sild-banner--success{--_bg:var(--success-subtle);--_bd:#B6E6C9}
.sild-banner--warning{--_bg:var(--warning-subtle);--_bd:#F5DCA6}
.sild-banner--danger{--_bg:var(--danger-subtle);--_bd:#F4C2C3}
.sild-banner__icon{flex:none;margin-top:1px}
.sild-banner--info .sild-banner__icon{color:var(--blue-600)}
.sild-banner--success .sild-banner__icon{color:var(--green-600)}
.sild-banner--warning .sild-banner__icon{color:var(--amber-600)}
.sild-banner--danger .sild-banner__icon{color:var(--red-600)}
.sild-banner__body{flex:1;min-width:0}
.sild-banner__title{font-size:14px;font-weight:600;line-height:1.4}
.sild-banner__msg{font-size:13px;color:var(--text-secondary);line-height:1.5;margin-top:2px}
.sild-banner__close{flex:none;background:none;border:0;cursor:pointer;color:var(--text-tertiary);padding:2px;border-radius:var(--radius-xs)}
.sild-banner__close:hover{color:var(--text-primary);background:rgba(0,0,0,.05)}
`;
  document.head.appendChild(s);
}
const ICONS = {
  info: 'M12 16v-4M12 8h.01M12 22a10 10 0 100-20 10 10 0 000 20z',
  success: 'M22 11.08V12a10 10 0 11-5.93-9.14M22 4 12 14.01l-3-3',
  warning: 'M10.29 3.86 1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0zM12 9v4M12 17h.01',
  danger: 'M12 8v4M12 16h.01M12 22a10 10 0 100-20 10 10 0 000 20z'
};
function Banner({
  variant = 'info',
  title,
  onClose,
  className = '',
  children,
  ...rest
}) {
  injectCss();
  return /*#__PURE__*/React.createElement("div", _extends({
    className: ['sild-banner', `sild-banner--${variant}`, className].filter(Boolean).join(' '),
    role: "status"
  }, rest), /*#__PURE__*/React.createElement("span", {
    className: "sild-banner__icon"
  }, /*#__PURE__*/React.createElement("svg", {
    width: "18",
    height: "18",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }, /*#__PURE__*/React.createElement("path", {
    d: ICONS[variant]
  }))), /*#__PURE__*/React.createElement("div", {
    className: "sild-banner__body"
  }, title && /*#__PURE__*/React.createElement("div", {
    className: "sild-banner__title"
  }, title), children && /*#__PURE__*/React.createElement("div", {
    className: "sild-banner__msg"
  }, children)), onClose && /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: "sild-banner__close",
    "aria-label": "Dismiss",
    onClick: onClose
  }, /*#__PURE__*/React.createElement("svg", {
    width: "15",
    height: "15",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M18 6 6 18M6 6l12 12"
  }))));
}
Object.assign(__ds_scope, { Banner });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Banner.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Card.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-card{background:var(--surface-card);border:1px solid var(--border-default);border-radius:var(--radius-lg);
  box-shadow:var(--shadow-sm);overflow:hidden;font-family:var(--font-sans)}
.sild-card--flat{box-shadow:none}
.sild-card--hover{transition:box-shadow var(--duration-base) var(--ease-standard),border-color var(--duration-base),transform var(--duration-base)}
.sild-card--hover:hover{box-shadow:var(--shadow-md);border-color:var(--border-strong)}
.sild-card__header{padding:16px 18px;border-bottom:1px solid var(--border-subtle);display:flex;align-items:center;justify-content:space-between;gap:12px}
.sild-card__title{font-size:15px;font-weight:700;color:var(--text-primary);letter-spacing:-.01em}
.sild-card__sub{font-size:13px;color:var(--text-tertiary);margin-top:2px}
.sild-card__body{padding:18px}
.sild-card__footer{padding:14px 18px;border-top:1px solid var(--border-subtle);background:var(--surface-page);display:flex;gap:10px;justify-content:flex-end}
`;
  document.head.appendChild(s);
}
function Card({
  title,
  subtitle,
  headerAction,
  footer,
  flat = false,
  hoverable = false,
  padded = true,
  className = '',
  children,
  ...rest
}) {
  injectCss();
  const cls = ['sild-card', flat ? 'sild-card--flat' : '', hoverable ? 'sild-card--hover' : '', className].filter(Boolean).join(' ');
  return /*#__PURE__*/React.createElement("div", _extends({
    className: cls
  }, rest), (title || headerAction) && /*#__PURE__*/React.createElement("div", {
    className: "sild-card__header"
  }, /*#__PURE__*/React.createElement("div", null, title && /*#__PURE__*/React.createElement("div", {
    className: "sild-card__title"
  }, title), subtitle && /*#__PURE__*/React.createElement("div", {
    className: "sild-card__sub"
  }, subtitle)), headerAction), padded ? /*#__PURE__*/React.createElement("div", {
    className: "sild-card__body"
  }, children) : children, footer && /*#__PURE__*/React.createElement("div", {
    className: "sild-card__footer"
  }, footer));
}
Object.assign(__ds_scope, { Card });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Card.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Dialog.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-dialog__scrim{position:fixed;inset:0;background:var(--surface-overlay);z-index:1000;
  display:flex;align-items:center;justify-content:center;padding:24px;
  animation:sild-fade var(--duration-base) var(--ease-standard)}
.sild-dialog{background:var(--surface-card);border-radius:var(--radius-xl);box-shadow:var(--shadow-xl);
  width:100%;max-width:var(--_w,480px);max-height:calc(100vh - 48px);display:flex;flex-direction:column;
  font-family:var(--font-sans);overflow:hidden;animation:sild-pop var(--duration-base) var(--ease-out)}
.sild-dialog__head{padding:20px 22px 0;display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.sild-dialog__title{font-size:18px;font-weight:700;letter-spacing:-.01em;color:var(--text-primary)}
.sild-dialog__sub{font-size:13px;color:var(--text-tertiary);margin-top:3px}
.sild-dialog__body{padding:16px 22px;overflow-y:auto}
.sild-dialog__foot{padding:14px 22px 20px;display:flex;gap:10px;justify-content:flex-end}
.sild-dialog__x{background:none;border:0;cursor:pointer;color:var(--text-tertiary);padding:4px;border-radius:var(--radius-sm)}
.sild-dialog__x:hover{color:var(--text-primary);background:var(--surface-hover)}
@keyframes sild-fade{from{opacity:0}to{opacity:1}}
@keyframes sild-pop{from{opacity:0;transform:translateY(8px) scale(.98)}to{opacity:1;transform:none}}
@media (prefers-reduced-motion:reduce){.sild-dialog,.sild-dialog__scrim{animation:none}}
`;
  document.head.appendChild(s);
}
function Dialog({
  open = true,
  onClose,
  title,
  subtitle,
  footer,
  width = 480,
  className = '',
  children,
  ...rest
}) {
  injectCss();
  if (!open) return null;
  return /*#__PURE__*/React.createElement("div", {
    className: "sild-dialog__scrim",
    onClick: e => {
      if (e.target === e.currentTarget && onClose) onClose();
    }
  }, /*#__PURE__*/React.createElement("div", _extends({
    className: ['sild-dialog', className].filter(Boolean).join(' '),
    role: "dialog",
    "aria-modal": "true",
    style: {
      '--_w': typeof width === 'number' ? width + 'px' : width
    }
  }, rest), (title || onClose) && /*#__PURE__*/React.createElement("div", {
    className: "sild-dialog__head"
  }, /*#__PURE__*/React.createElement("div", null, title && /*#__PURE__*/React.createElement("div", {
    className: "sild-dialog__title"
  }, title), subtitle && /*#__PURE__*/React.createElement("div", {
    className: "sild-dialog__sub"
  }, subtitle)), onClose && /*#__PURE__*/React.createElement("button", {
    type: "button",
    className: "sild-dialog__x",
    "aria-label": "Close",
    onClick: onClose
  }, /*#__PURE__*/React.createElement("svg", {
    width: "18",
    height: "18",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M18 6 6 18M6 6l12 12"
  })))), /*#__PURE__*/React.createElement("div", {
    className: "sild-dialog__body"
  }, children), footer && /*#__PURE__*/React.createElement("div", {
    className: "sild-dialog__foot"
  }, footer)));
}
Object.assign(__ds_scope, { Dialog });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Dialog.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Tooltip.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-tip-wrap{position:relative;display:inline-flex}
.sild-tip{position:absolute;z-index:50;background:var(--slate-900);color:#fff;font-family:var(--font-sans);
  font-size:12px;font-weight:500;line-height:1.4;padding:6px 9px;border-radius:var(--radius-sm);
  white-space:nowrap;box-shadow:var(--shadow-md);pointer-events:none;
  opacity:0;transform:translateY(2px);transition:opacity var(--duration-fast),transform var(--duration-fast)}
.sild-tip--show{opacity:1;transform:translateY(0)}
.sild-tip--top{bottom:calc(100% + 7px);left:50%;translate:-50% 0}
.sild-tip--bottom{top:calc(100% + 7px);left:50%;translate:-50% 0}
.sild-tip--left{right:calc(100% + 7px);top:50%;translate:0 -50%}
.sild-tip--right{left:calc(100% + 7px);top:50%;translate:0 -50%}
@media (prefers-reduced-motion:reduce){.sild-tip{transition:none}}
`;
  document.head.appendChild(s);
}
function Tooltip({
  content,
  side = 'top',
  className = '',
  children,
  ...rest
}) {
  injectCss();
  const [show, setShow] = React.useState(false);
  return /*#__PURE__*/React.createElement("span", _extends({
    className: ['sild-tip-wrap', className].filter(Boolean).join(' '),
    onMouseEnter: () => setShow(true),
    onMouseLeave: () => setShow(false),
    onFocus: () => setShow(true),
    onBlur: () => setShow(false)
  }, rest), children, /*#__PURE__*/React.createElement("span", {
    role: "tooltip",
    className: ['sild-tip', `sild-tip--${side}`, show ? 'sild-tip--show' : ''].filter(Boolean).join(' ')
  }, content));
}
Object.assign(__ds_scope, { Tooltip });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Tooltip.jsx", error: String((e && e.message) || e) }); }

// components/forms/Checkbox.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-check{display:inline-flex;align-items:flex-start;gap:9px;font-family:var(--font-sans);cursor:pointer;user-select:none}
.sild-check[aria-disabled="true"]{cursor:not-allowed;opacity:.55}
.sild-check__box{flex:none;width:18px;height:18px;border-radius:var(--radius-xs);border:1.5px solid var(--border-strong);
  background:var(--white);display:inline-flex;align-items:center;justify-content:center;color:#fff;margin-top:1px;
  transition:background var(--duration-fast),border-color var(--duration-fast)}
.sild-check input:focus-visible + .sild-check__box{box-shadow:var(--ring);border-color:var(--border-focus)}
.sild-check__box--on{background:var(--brand);border-color:var(--brand)}
.sild-check__txt{font-size:14px;color:var(--text-primary);line-height:1.35}
.sild-check__sub{font-size:12px;color:var(--text-tertiary);margin-top:1px}
.sild-check__hide{position:absolute;opacity:0;width:0;height:0;pointer-events:none}
`;
  document.head.appendChild(s);
}
function Checkbox({
  checked = false,
  onChange,
  label,
  description,
  disabled = false,
  className = '',
  ...rest
}) {
  injectCss();
  return /*#__PURE__*/React.createElement("label", {
    className: ['sild-check', className].filter(Boolean).join(' '),
    "aria-disabled": disabled
  }, /*#__PURE__*/React.createElement("input", _extends({
    type: "checkbox",
    className: "sild-check__hide",
    checked: checked,
    disabled: disabled,
    onChange: e => onChange && onChange(e.target.checked, e)
  }, rest)), /*#__PURE__*/React.createElement("span", {
    className: ['sild-check__box', checked ? 'sild-check__box--on' : ''].filter(Boolean).join(' ')
  }, checked && /*#__PURE__*/React.createElement("svg", {
    width: "12",
    height: "12",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "3.2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M20 6 9 17l-5-5"
  }))), (label || description) && /*#__PURE__*/React.createElement("span", null, label && /*#__PURE__*/React.createElement("span", {
    className: "sild-check__txt"
  }, label), description && /*#__PURE__*/React.createElement("span", {
    className: "sild-check__sub"
  }, description)));
}
Object.assign(__ds_scope, { Checkbox });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Checkbox.jsx", error: String((e && e.message) || e) }); }

// components/forms/Input.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-field{display:flex;flex-direction:column;gap:6px;font-family:var(--font-sans)}
.sild-field__label{font-size:13px;font-weight:600;color:var(--text-primary)}
.sild-field__req{color:var(--danger);margin-left:2px}
.sild-field__hint{font-size:12px;color:var(--text-tertiary)}
.sild-field__hint--error{color:var(--danger)}
.sild-input{display:flex;align-items:center;gap:8px;background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-md);transition:border-color var(--duration-fast),box-shadow var(--duration-fast);padding:0 12px}
.sild-input:focus-within{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-input--error{border-color:var(--danger)}
.sild-input--error:focus-within{box-shadow:var(--ring-danger)}
.sild-input--disabled{background:var(--surface-sunken);opacity:.7;cursor:not-allowed}
.sild-input--sm{height:34px}
.sild-input--md{height:40px}
.sild-input--lg{height:46px}
.sild-input__el{flex:1;border:0;outline:none;background:transparent;font-family:inherit;font-size:14px;
  color:var(--text-primary);min-width:0;height:100%}
.sild-input__el::placeholder{color:var(--text-tertiary)}
.sild-input__icon{display:inline-flex;color:var(--text-tertiary);flex:none}
`;
  document.head.appendChild(s);
}
function Input({
  label,
  hint,
  error,
  required = false,
  size = 'md',
  iconLeft = null,
  iconRight = null,
  disabled = false,
  className = '',
  id,
  ...rest
}) {
  injectCss();
  const fid = id || (label ? 'in-' + label.replace(/\s+/g, '-').toLowerCase() : undefined);
  const wrapCls = ['sild-input', `sild-input--${size}`, error ? 'sild-input--error' : '', disabled ? 'sild-input--disabled' : ''].filter(Boolean).join(' ');
  return /*#__PURE__*/React.createElement("div", {
    className: ['sild-field', className].filter(Boolean).join(' ')
  }, label && /*#__PURE__*/React.createElement("label", {
    className: "sild-field__label",
    htmlFor: fid
  }, label, required && /*#__PURE__*/React.createElement("span", {
    className: "sild-field__req"
  }, "*")), /*#__PURE__*/React.createElement("div", {
    className: wrapCls
  }, iconLeft && /*#__PURE__*/React.createElement("span", {
    className: "sild-input__icon"
  }, iconLeft), /*#__PURE__*/React.createElement("input", _extends({
    id: fid,
    className: "sild-input__el",
    disabled: disabled,
    "aria-invalid": !!error
  }, rest)), iconRight && /*#__PURE__*/React.createElement("span", {
    className: "sild-input__icon"
  }, iconRight)), (error || hint) && /*#__PURE__*/React.createElement("span", {
    className: ['sild-field__hint', error ? 'sild-field__hint--error' : ''].filter(Boolean).join(' ')
  }, error || hint));
}
Object.assign(__ds_scope, { Input });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Input.jsx", error: String((e && e.message) || e) }); }

// components/forms/Select.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-select{position:relative;display:flex;align-items:center;background:var(--white);
  border:1px solid var(--border-default);border-radius:var(--radius-md);transition:border-color var(--duration-fast),box-shadow var(--duration-fast)}
.sild-select:focus-within{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-select--sm{height:34px}.sild-select--md{height:40px}.sild-select--lg{height:46px}
.sild-select__el{appearance:none;-webkit-appearance:none;border:0;outline:none;background:transparent;
  font-family:var(--font-sans);font-size:14px;color:var(--text-primary);height:100%;width:100%;
  padding:0 36px 0 12px;cursor:pointer}
.sild-select__el[disabled]{cursor:not-allowed;color:var(--text-disabled)}
.sild-select__chev{position:absolute;right:12px;pointer-events:none;color:var(--text-tertiary)}
`;
  document.head.appendChild(s);
}
function Select({
  label,
  hint,
  error,
  required = false,
  size = 'md',
  options = [],
  placeholder,
  className = '',
  id,
  children,
  ...rest
}) {
  injectCss();
  const fid = id || (label ? 'sel-' + label.replace(/\s+/g, '-').toLowerCase() : undefined);
  return /*#__PURE__*/React.createElement("div", {
    className: ['sild-field', className].filter(Boolean).join(' ')
  }, label && /*#__PURE__*/React.createElement("label", {
    className: "sild-field__label",
    htmlFor: fid
  }, label, required && /*#__PURE__*/React.createElement("span", {
    className: "sild-field__req"
  }, "*")), /*#__PURE__*/React.createElement("div", {
    className: ['sild-select', `sild-select--${size}`].filter(Boolean).join(' '),
    style: error ? {
      borderColor: 'var(--danger)'
    } : undefined
  }, /*#__PURE__*/React.createElement("select", _extends({
    id: fid,
    className: "sild-select__el"
  }, rest), placeholder && /*#__PURE__*/React.createElement("option", {
    value: "",
    disabled: true
  }, placeholder), options.map(o => {
    const val = typeof o === 'string' ? o : o.value;
    const lab = typeof o === 'string' ? o : o.label;
    return /*#__PURE__*/React.createElement("option", {
      key: val,
      value: val
    }, lab);
  }), children), /*#__PURE__*/React.createElement("span", {
    className: "sild-select__chev"
  }, /*#__PURE__*/React.createElement("svg", {
    width: "16",
    height: "16",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M6 9l6 6 6-6"
  })))), (error || hint) && /*#__PURE__*/React.createElement("span", {
    className: ['sild-field__hint', error ? 'sild-field__hint--error' : ''].filter(Boolean).join(' ')
  }, error || hint));
}
Object.assign(__ds_scope, { Select });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Select.jsx", error: String((e && e.message) || e) }); }

// components/forms/Switch.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-switch{display:inline-flex;align-items:center;gap:10px;font-family:var(--font-sans);cursor:pointer;user-select:none}
.sild-switch[aria-disabled="true"]{cursor:not-allowed;opacity:.55}
.sild-switch__track{position:relative;flex:none;width:36px;height:20px;border-radius:var(--radius-full);
  background:var(--slate-300);transition:background var(--duration-base) var(--ease-standard)}
.sild-switch__track--on{background:var(--brand)}
.sild-switch__knob{position:absolute;top:2px;left:2px;width:16px;height:16px;border-radius:50%;background:#fff;
  box-shadow:var(--shadow-xs);transition:transform var(--duration-base) var(--ease-standard)}
.sild-switch__track--on .sild-switch__knob{transform:translateX(16px)}
.sild-switch input:focus-visible + .sild-switch__track{box-shadow:var(--ring)}
.sild-switch__txt{font-size:14px;color:var(--text-primary)}
.sild-switch__hide{position:absolute;opacity:0;width:0;height:0}
@media (prefers-reduced-motion:reduce){.sild-switch__track,.sild-switch__knob{transition:none}}
`;
  document.head.appendChild(s);
}
function Switch({
  checked = false,
  onChange,
  label,
  disabled = false,
  className = '',
  ...rest
}) {
  injectCss();
  return /*#__PURE__*/React.createElement("label", {
    className: ['sild-switch', className].filter(Boolean).join(' '),
    "aria-disabled": disabled
  }, /*#__PURE__*/React.createElement("input", _extends({
    type: "checkbox",
    role: "switch",
    className: "sild-switch__hide",
    checked: checked,
    disabled: disabled,
    onChange: e => onChange && onChange(e.target.checked, e)
  }, rest)), /*#__PURE__*/React.createElement("span", {
    className: ['sild-switch__track', checked ? 'sild-switch__track--on' : ''].filter(Boolean).join(' ')
  }, /*#__PURE__*/React.createElement("span", {
    className: "sild-switch__knob"
  })), label && /*#__PURE__*/React.createElement("span", {
    className: "sild-switch__txt"
  }, label));
}
Object.assign(__ds_scope, { Switch });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Switch.jsx", error: String((e && e.message) || e) }); }

// components/forms/Textarea.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
let _injected = false;
function injectCss() {
  if (_injected || typeof document === 'undefined') return;
  _injected = true;
  const s = document.createElement('style');
  s.textContent = `
.sild-textarea{width:100%;font-family:var(--font-sans);font-size:14px;color:var(--text-primary);
  background:var(--white);border:1px solid var(--border-default);border-radius:var(--radius-md);
  padding:10px 12px;resize:vertical;line-height:1.5;transition:border-color var(--duration-fast),box-shadow var(--duration-fast);outline:none}
.sild-textarea::placeholder{color:var(--text-tertiary)}
.sild-textarea:focus{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-textarea--error{border-color:var(--danger)}
.sild-textarea--error:focus{box-shadow:var(--ring-danger)}
.sild-textarea[disabled]{background:var(--surface-sunken);opacity:.7}
`;
  document.head.appendChild(s);
}
function Textarea({
  label,
  hint,
  error,
  required = false,
  rows = 4,
  className = '',
  id,
  ...rest
}) {
  injectCss();
  const fid = id || (label ? 'ta-' + label.replace(/\s+/g, '-').toLowerCase() : undefined);
  return /*#__PURE__*/React.createElement("div", {
    className: ['sild-field', className].filter(Boolean).join(' ')
  }, label && /*#__PURE__*/React.createElement("label", {
    className: "sild-field__label",
    htmlFor: fid
  }, label, required && /*#__PURE__*/React.createElement("span", {
    className: "sild-field__req"
  }, "*")), /*#__PURE__*/React.createElement("textarea", _extends({
    id: fid,
    rows: rows,
    className: ['sild-textarea', error ? 'sild-textarea--error' : ''].filter(Boolean).join(' '),
    "aria-invalid": !!error
  }, rest)), (error || hint) && /*#__PURE__*/React.createElement("span", {
    className: ['sild-field__hint', error ? 'sild-field__hint--error' : ''].filter(Boolean).join(' ')
  }, error || hint));
}
Object.assign(__ds_scope, { Textarea });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Textarea.jsx", error: String((e && e.message) || e) }); }

// ui_kits/icons.jsx
try { (() => {
/* Lucide-style line icons (2px, round caps) as React components for the Sild UI kits.
   Substituted from the Lucide set; see readme ICONOGRAPHY. */
(function () {
  const React = window.React;
  const S = (paths, props = {}) => extra => React.createElement('svg', {
    width: 20,
    height: 20,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 2,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
    ...props,
    ...(extra || {})
  }, paths.map((d, i) => React.createElement('path', {
    key: i,
    d
  })));
  const C = (children, props = {}) => extra => React.createElement('svg', {
    width: 20,
    height: 20,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 2,
    strokeLinecap: 'round',
    strokeLinejoin: 'round',
    ...props,
    ...(extra || {})
  }, children);
  const h = React.createElement;
  window.I = {
    Inbox: S(['M22 12h-6l-2 3h-4l-2-3H2', 'M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z']),
    Search: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('circle', {
      key: 0,
      cx: 11,
      cy: 11,
      r: 8
    }), h('path', {
      key: 1,
      d: 'm21 21-4.3-4.3'
    })]),
    Settings: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('circle', {
      key: 0,
      cx: 12,
      cy: 12,
      r: 3
    }), h('path', {
      key: 1,
      d: 'M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z'
    })]),
    Bell: S(['M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9', 'M10.3 21a1.94 1.94 0 0 0 3.4 0']),
    Mail: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('rect', {
      key: 0,
      x: 2,
      y: 4,
      width: 20,
      height: 16,
      rx: 2
    }), h('path', {
      key: 1,
      d: 'm22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7'
    })]),
    Phone: S(['M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.13.96.36 1.9.7 2.81a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.9.34 1.85.57 2.81.7A2 2 0 0 1 22 16.92z']),
    Lock: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('rect', {
      key: 0,
      x: 3,
      y: 11,
      width: 18,
      height: 11,
      rx: 2
    }), h('path', {
      key: 1,
      d: 'M7 11V7a5 5 0 0 1 10 0v4'
    })]),
    Plus: S(['M5 12h14', 'M12 5v14']),
    Check: S(['M20 6 9 17l-5-5']),
    X: S(['M18 6 6 18', 'M6 6l12 12']),
    Users: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('path', {
      key: 0,
      d: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2'
    }), h('circle', {
      key: 1,
      cx: 9,
      cy: 7,
      r: 4
    }), h('path', {
      key: 2,
      d: 'M22 21v-2a4 4 0 0 0-3-3.87'
    }), h('path', {
      key: 3,
      d: 'M16 3.13a4 4 0 0 1 0 7.75'
    })]),
    Key: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('circle', {
      key: 0,
      cx: 7.5,
      cy: 15.5,
      r: 5.5
    }), h('path', {
      key: 1,
      d: 'm21 2-9.6 9.6'
    }), h('path', {
      key: 2,
      d: 'm15.5 7.5 3 3L22 7l-3-3'
    })]),
    Webhook: S(['M18 16.98h-5.99c-1.1 0-1.95.94-2.48 1.9A4 4 0 0 1 2 17c.01-.7.2-1.4.57-2', 'M6 17l3.13-5.78c.53-.97.1-2.18-.5-3.1a4 4 0 1 1 6.89-4.06', 'M12 6l3.13 5.73C15.66 12.7 16.9 13 18 13a4 4 0 0 1 0 8']),
    More: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('circle', {
      key: 0,
      cx: 12,
      cy: 12,
      r: 1
    }), h('circle', {
      key: 1,
      cx: 19,
      cy: 12,
      r: 1
    }), h('circle', {
      key: 2,
      cx: 5,
      cy: 12,
      r: 1
    })]),
    Filter: S(['M22 3H2l8 9.46V19l4 2v-8.54L22 3z']),
    Phone2: S(['M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2']),
    Smartphone: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('rect', {
      key: 0,
      x: 5,
      y: 2,
      width: 14,
      height: 20,
      rx: 2.5
    }), h('path', {
      key: 1,
      d: 'M12 18h.01'
    })]),
    Copy: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('rect', {
      key: 0,
      x: 9,
      y: 9,
      width: 13,
      height: 13,
      rx: 2
    }), h('path', {
      key: 1,
      d: 'M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1'
    })]),
    Hash: S(['M4 9h16', 'M4 15h16', 'M10 3 8 21', 'M16 3l-2 18']),
    PanelRight: e => h('svg', {
      width: 20,
      height: 20,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
      ...(e || {})
    }, [h('rect', {
      key: 0,
      x: 3,
      y: 3,
      width: 18,
      height: 18,
      rx: 2
    }), h('path', {
      key: 1,
      d: 'M15 3v18'
    })])
  };
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/icons.jsx", error: String((e && e.message) || e) }); }

// ui_kits/inbox/App.jsx
try { (() => {
/* Sild Settings (spec §8: API keys, Webhooks, Team) + root App. */
const {
  useState: uS
} = React;
const SD = window.Sild,
  IK = window.I,
  DT = window.SILD_DATA;
const {
  Button: B2,
  IconButton: IB2,
  Avatar: A2,
  Badge: BG2,
  Tag: TG2,
  Input: IN2,
  Select: SE2,
  Switch: SW2,
  Card: CD2,
  Banner: BN2,
  Tooltip: TP2,
  Dialog: DG2
} = SD;
const {
  Login: Login2,
  NavRail: NavRail2
} = window.InboxParts;
const {
  ConvList: CL,
  MemberPanel: MP,
  ConvView: CV
} = window.InboxMain;
const SectionTitle = ({
  children
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    fontSize: 11,
    fontWeight: 600,
    letterSpacing: '.04em',
    textTransform: 'uppercase',
    color: 'var(--text-tertiary)'
  }
}, children);
function Settings() {
  const [tab, setTab] = uS('keys');
  const [keyDialog, setKeyDialog] = uS(false);
  const [hooks, setHooks] = uS([{
    url: 'https://api.acme.com/sild/hooks',
    events: ['message.created', 'conversation.closed'],
    active: true
  }, {
    url: 'https://ops.acme.com/ingest',
    events: ['assignment.updated'],
    active: false
  }]);
  const tabs = [['keys', 'API keys', IK.Key], ['hooks', 'Webhooks', IK.Webhook], ['team', 'Team', IK.Users]];
  return /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      overflowY: 'auto',
      background: 'var(--surface-page)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 760,
      margin: '0 auto',
      padding: '32px 28px 64px'
    }
  }, /*#__PURE__*/React.createElement("h1", {
    style: {
      fontSize: 26
    }
  }, "Settings"), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 4,
      marginTop: 18,
      borderBottom: '1px solid var(--border-default)'
    }
  }, tabs.map(([k, l, icon]) => /*#__PURE__*/React.createElement("button", {
    key: k,
    onClick: () => setTab(k),
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 7,
      border: 0,
      background: 'transparent',
      cursor: 'pointer',
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      fontWeight: 600,
      padding: '10px 12px',
      marginBottom: -1,
      color: tab === k ? 'var(--brand)' : 'var(--text-secondary)',
      borderBottom: tab === k ? '2px solid var(--brand)' : '2px solid transparent'
    }
  }, icon({
    width: 17,
    height: 17
  }), l))), /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 24
    }
  }, tab === 'keys' && /*#__PURE__*/React.createElement(CD2, {
    title: "API keys",
    subtitle: "Server-to-server credentials. Each key is shown once at creation.",
    headerAction: /*#__PURE__*/React.createElement(B2, {
      size: "sm",
      iconLeft: IK.Plus({
        width: 16,
        height: 16
      }),
      onClick: () => setKeyDialog(true)
    }, "New key"),
    padded: false
  }, [['Production backend', 'sild_live_3xK9…dH1j', 'Mar 4, 2026'], ['WordPress plugin', 'sild_live_8aQ2…pL0v', 'Feb 19, 2026']].map((r, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '14px 18px',
      borderTop: '1px solid var(--border-subtle)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 14,
      fontWeight: 600
    }
  }, r[0]), /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      color: 'var(--text-tertiary)',
      marginTop: 2
    }
  }, r[1])), /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 12,
      color: 'var(--text-tertiary)'
    }
  }, r[2]), /*#__PURE__*/React.createElement(B2, {
    size: "sm",
    variant: "ghost"
  }, "Revoke")))), tab === 'hooks' && /*#__PURE__*/React.createElement(CD2, {
    title: "Webhooks",
    subtitle: "We POST events with an HMAC signature. Consumers dedupe on the event id.",
    headerAction: /*#__PURE__*/React.createElement(B2, {
      size: "sm",
      iconLeft: IK.Plus({
        width: 16,
        height: 16
      })
    }, "Add endpoint"),
    padded: false
  }, hooks.map((hk, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '14px 18px',
      borderTop: '1px solid var(--border-subtle)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      color: 'var(--text-primary)',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, hk.url), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 5,
      marginTop: 7,
      flexWrap: 'wrap'
    }
  }, hk.events.map(e => /*#__PURE__*/React.createElement(TG2, {
    key: e,
    mono: true
  }, e)))), /*#__PURE__*/React.createElement(SW2, {
    checked: hk.active,
    onChange: v => setHooks(hs => hs.map((x, j) => j === i ? {
      ...x,
      active: v
    } : x))
  }), /*#__PURE__*/React.createElement(IB2, {
    size: "sm",
    "aria-label": "Delete endpoint",
    onClick: () => setHooks(hs => hs.filter((_, j) => j !== i))
  }, IK.X({
    width: 18,
    height: 18
  }))))), tab === 'team' && /*#__PURE__*/React.createElement(CD2, {
    title: "Team",
    subtitle: "Invite agents and set platform roles (owner / admin / agent).",
    headerAction: /*#__PURE__*/React.createElement(B2, {
      size: "sm",
      iconLeft: IK.Plus({
        width: 16,
        height: 16
      })
    }, "Invite agent"),
    padded: false
  }, [['Liis Mägi', 'liis@sild.io', 'owner'], ['Tomas Rebane', 'tomas@sild.io', 'admin'], ['Eva Lill', 'eva@sild.io', 'agent']].map((m, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '12px 18px',
      borderTop: '1px solid var(--border-subtle)'
    }
  }, /*#__PURE__*/React.createElement(A2, {
    name: m[0],
    size: "md"
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 14,
      fontWeight: 600
    }
  }, m[0]), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 12,
      color: 'var(--text-tertiary)'
    }
  }, m[1])), /*#__PURE__*/React.createElement("div", {
    style: {
      width: 130
    }
  }, /*#__PURE__*/React.createElement(SE2, {
    size: "sm",
    defaultValue: m[2],
    options: [{
      label: 'Owner',
      value: 'owner'
    }, {
      label: 'Admin',
      value: 'admin'
    }, {
      label: 'Agent',
      value: 'agent'
    }]
  }))))))), /*#__PURE__*/React.createElement(DG2, {
    open: keyDialog,
    onClose: () => setKeyDialog(false),
    title: "API key created",
    subtitle: "Copy it now \u2014 it won't be shown again.",
    footer: /*#__PURE__*/React.createElement(B2, {
      onClick: () => setKeyDialog(false)
    }, "Done")
  }, /*#__PURE__*/React.createElement(BN2, {
    variant: "warning",
    title: "Shown once"
  }, "Store this key in your backend secrets. Sild only keeps a hash."), /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 14,
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      background: 'var(--surface-sunken)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-md)',
      padding: '10px 12px'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      flex: 1,
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      wordBreak: 'break-all'
    }
  }, "sild_live_3xK9b2pQ7mWnL4vR8sT0aF6dH1jE5cY2"), /*#__PURE__*/React.createElement(IB2, {
    size: "sm",
    variant: "bordered",
    "aria-label": "Copy"
  }, IK.Copy({
    width: 16,
    height: 16
  })))));
}

/* ---------------- Root app ---------------- */
function App() {
  const [signedIn, setSignedIn] = uS(false);
  const [view, setView] = uS('inbox');
  const [convs, setConvs] = uS(DT.conversations.map(c => ({
    ...c,
    messages: [...c.messages]
  })));
  const [activeId, setActiveId] = uS(DT.conversations[0].id);
  const [filter, setFilter] = uS('all');
  const [panelOpen, setPanelOpen] = uS(true);
  const active = convs.find(c => c.id === activeId) || convs[0];
  const mutate = (id, fn) => setConvs(cs => cs.map(c => c.id === id ? fn(c) : c));
  const send = (text, internal) => mutate(activeId, c => ({
    ...c,
    preview: internal ? c.preview : text,
    unread: 0,
    messages: [...c.messages, {
      id: Date.now(),
      dir: 'out',
      author: 'You',
      time: 'now',
      body: text,
      internal,
      read: internal ? null : 'Sent'
    }]
  }));
  const claim = () => mutate(activeId, c => ({
    ...c,
    status: 'assigned'
  }));
  const closeConv = () => mutate(activeId, c => ({
    ...c,
    status: 'closed',
    messages: [...c.messages, {
      id: Date.now(),
      system: true,
      body: `Conversation closed by ${DT.agent.name}`
    }]
  }));
  if (!signedIn) return /*#__PURE__*/React.createElement(Login2, {
    onSignIn: () => setSignedIn(true)
  });
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      display: 'flex',
      fontFamily: 'var(--font-sans)'
    }
  }, /*#__PURE__*/React.createElement(NavRail2, {
    view: view,
    setView: setView,
    onSignOut: () => setSignedIn(false)
  }), view === 'inbox' ? /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement(CL, {
    convs: convs,
    activeId: activeId,
    setActiveId: setActiveId,
    filter: filter,
    setFilter: setFilter,
    onNew: () => {}
  }), /*#__PURE__*/React.createElement(CV, {
    conv: active,
    onSend: send,
    onClaim: claim,
    onClose: closeConv,
    panelOpen: panelOpen,
    setPanelOpen: setPanelOpen
  }), panelOpen && /*#__PURE__*/React.createElement(MP, {
    conv: active,
    onClose: () => setPanelOpen(false)
  })) : /*#__PURE__*/React.createElement(Settings, null));
}
ReactDOM.createRoot(document.getElementById('root')).render(/*#__PURE__*/React.createElement(App, null));
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/inbox/App.jsx", error: String((e && e.message) || e) }); }

// ui_kits/inbox/Inbox.jsx
try { (() => {
/* Sild Support Inbox — main screens + root app. */
const {
  useState: useS,
  useRef: useR,
  useEffect: useE
} = React;
const SS = window.Sild,
  IC = window.I,
  D = window.SILD_DATA;
const {
  Button: Btn,
  IconButton: IBtn,
  Avatar: Av,
  Badge: Bdg,
  Tag: Tg,
  Input: Inp,
  Select: Sel,
  Switch: Sw,
  Card: Crd,
  Banner: Bnr,
  Tooltip: Tip,
  Dialog: Dlg,
  StatusPill: SP,
  MessageBubble: MB,
  ConversationRow: CR,
  ComposerBar: CB
} = SS;
const {
  Login: LoginS,
  NavRail: NavRailS
} = window.InboxParts;

/* ---------- Conversation list ---------- */
function ConvList({
  convs,
  activeId,
  setActiveId,
  filter,
  setFilter,
  onNew
}) {
  const tabs = [['you', 'You'], ['unassigned', 'Unassigned'], ['all', 'All']];
  const filtered = convs.filter(c => filter === 'all' ? true : filter === 'unassigned' ? c.status === 'queued' : c.status !== 'closed');
  return /*#__PURE__*/React.createElement("div", {
    style: {
      width: 360,
      flex: 'none',
      borderRight: '1px solid var(--border-default)',
      background: 'var(--surface-card)',
      display: 'flex',
      flexDirection: 'column'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      padding: '16px 16px 12px'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between'
    }
  }, /*#__PURE__*/React.createElement("h2", {
    style: {
      fontSize: 18
    }
  }, "Inbox"), /*#__PURE__*/React.createElement(Btn, {
    size: "sm",
    iconLeft: IC.Plus({
      width: 16,
      height: 16
    }),
    onClick: onNew
  }, "New request")), /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 12
    }
  }, /*#__PURE__*/React.createElement(Inp, {
    iconLeft: IC.Search({
      width: 16,
      height: 16
    }),
    placeholder: "Search \u2014 try status:open or a phone number",
    size: "sm"
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 4,
      marginTop: 12,
      background: 'var(--surface-sunken)',
      padding: 3,
      borderRadius: 'var(--radius-md)'
    }
  }, tabs.map(([k, l]) => /*#__PURE__*/React.createElement("button", {
    key: k,
    onClick: () => setFilter(k),
    style: {
      flex: 1,
      border: 0,
      cursor: 'pointer',
      fontFamily: 'var(--font-sans)',
      fontSize: 13,
      fontWeight: 600,
      padding: '6px 0',
      borderRadius: 'var(--radius-sm)',
      color: filter === k ? 'var(--text-primary)' : 'var(--text-secondary)',
      background: filter === k ? 'var(--white)' : 'transparent',
      boxShadow: filter === k ? 'var(--shadow-xs)' : 'none'
    }
  }, l)))), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      overflowY: 'auto',
      borderTop: '1px solid var(--border-subtle)'
    }
  }, filtered.map(c => /*#__PURE__*/React.createElement(CR, {
    key: c.id,
    name: c.name,
    preview: c.preview,
    time: c.time,
    unread: c.unread,
    channel: c.channel,
    reference: c.reference,
    presence: c.presence,
    active: c.id === activeId,
    onClick: () => setActiveId(c.id),
    status: c.status === 'queued' ? /*#__PURE__*/React.createElement(SP, {
      status: "queued"
    }) : null
  })), filtered.length === 0 && /*#__PURE__*/React.createElement("div", {
    style: {
      padding: '40px 24px',
      textAlign: 'center',
      color: 'var(--text-tertiary)',
      fontSize: 13,
      lineHeight: 1.6
    }
  }, "No conversations in this view. New support requests land here the moment they're assigned.")));
}

/* ---------- Member panel ---------- */
function MemberPanel({
  conv,
  onClose
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      width: 320,
      flex: 'none',
      borderLeft: '1px solid var(--border-default)',
      background: 'var(--surface-card)',
      display: 'flex',
      flexDirection: 'column'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      padding: '14px 16px',
      borderBottom: '1px solid var(--border-subtle)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 13,
      fontWeight: 700,
      letterSpacing: '-.01em'
    }
  }, "Details"), /*#__PURE__*/React.createElement(IBtn, {
    size: "sm",
    "aria-label": "Close panel",
    onClick: onClose
  }, IC.X({
    width: 18,
    height: 18
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: 16,
      overflowY: 'auto',
      display: 'flex',
      flexDirection: 'column',
      gap: 18
    }
  }, /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 11,
      fontWeight: 600,
      letterSpacing: '.04em',
      textTransform: 'uppercase',
      color: 'var(--text-tertiary)',
      marginBottom: 8
    }
  }, "Assignment"), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8
    }
  }, /*#__PURE__*/React.createElement(SP, {
    status: conv.status
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 13,
      color: 'var(--text-secondary)'
    }
  }, conv.status === 'queued' ? 'Unclaimed' : conv.status === 'closed' ? 'Closed' : 'You')), /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 8,
      display: 'flex',
      alignItems: 'center',
      gap: 6
    }
  }, /*#__PURE__*/React.createElement(Tg, {
    mono: true
  }, conv.reference), /*#__PURE__*/React.createElement(Tg, {
    mono: true
  }, conv.channel === 'email' ? 'channel:email' : 'channel:app'))), /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 11,
      fontWeight: 600,
      letterSpacing: '.04em',
      textTransform: 'uppercase',
      color: 'var(--text-tertiary)',
      marginBottom: 8
    }
  }, "Members (", conv.members.length, ")"), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 12
    }
  }, conv.members.map((m, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      display: 'flex',
      gap: 10
    }
  }, /*#__PURE__*/React.createElement(Av, {
    name: m.name,
    size: "md"
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      minWidth: 0,
      flex: 1
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 6
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 14,
      fontWeight: 600
    }
  }, m.name), /*#__PURE__*/React.createElement(Bdg, {
    variant: "neutral"
  }, m.role)), /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 6,
      display: 'flex',
      flexDirection: 'column',
      gap: 4
    }
  }, Object.entries(m.meta).map(([k, v]) => /*#__PURE__*/React.createElement("div", {
    key: k,
    style: {
      display: 'flex',
      gap: 6,
      fontSize: 12
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      color: 'var(--text-tertiary)',
      whiteSpace: 'nowrap'
    }
  }, k), /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--text-secondary)',
      wordBreak: 'break-all'
    }
  }, v)))))))))));
}

/* ---------- Conversation view ---------- */
function ConvView({
  conv,
  onSend,
  onClaim,
  onClose,
  panelOpen,
  setPanelOpen
}) {
  const [text, setText] = useS('');
  const [internal, setInternal] = useS(false);
  const scroller = useR(null);
  useE(() => {
    if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [conv.id, conv.messages.length]);
  if (!conv) return /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1
    }
  });
  const closed = conv.status === 'closed';
  return /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0,
      display: 'flex',
      flexDirection: 'column',
      background: 'var(--surface-page)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      height: 64,
      flex: 'none',
      padding: '0 18px',
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      background: 'var(--surface-card)',
      borderBottom: '1px solid var(--border-default)'
    }
  }, /*#__PURE__*/React.createElement(Av, {
    name: conv.name,
    presence: conv.presence,
    size: "md"
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 15,
      fontWeight: 700,
      letterSpacing: '-.01em'
    }
  }, conv.name), conv.channel === 'email' && /*#__PURE__*/React.createElement(Bdg, {
    variant: "brand"
  }, "Email")), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 12,
      color: 'var(--text-tertiary)',
      fontFamily: 'var(--font-mono)'
    }
  }, conv.reference)), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1
    }
  }), /*#__PURE__*/React.createElement(SP, {
    status: conv.status
  }), conv.status === 'queued' && /*#__PURE__*/React.createElement(Btn, {
    size: "sm",
    onClick: onClaim
  }, "Claim"), !closed && /*#__PURE__*/React.createElement(Btn, {
    size: "sm",
    variant: "secondary",
    onClick: onClose
  }, "Close conversation"), /*#__PURE__*/React.createElement(Tip, {
    content: panelOpen ? 'Hide details' : 'Show details'
  }, /*#__PURE__*/React.createElement(IBtn, {
    "aria-label": "Toggle details",
    variant: panelOpen ? 'bordered' : 'ghost',
    onClick: () => setPanelOpen(!panelOpen)
  }, IC.PanelRight({
    width: 20,
    height: 20
  })))), /*#__PURE__*/React.createElement("div", {
    ref: scroller,
    style: {
      flex: 1,
      overflowY: 'auto',
      padding: '20px 22px',
      display: 'flex',
      flexDirection: 'column',
      gap: 14
    }
  }, conv.messages.map(m => /*#__PURE__*/React.createElement(MB, {
    key: m.id,
    direction: m.dir,
    author: m.author,
    time: m.time,
    body: m.body,
    channel: m.channel,
    internal: m.internal,
    system: m.system,
    readReceipt: m.read
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 'none',
      padding: '12px 18px 16px',
      background: 'var(--surface-card)',
      borderTop: '1px solid var(--border-default)'
    }
  }, closed ? /*#__PURE__*/React.createElement(Bnr, {
    variant: "info"
  }, "This conversation is closed. Closed is terminal \u2014 open a new support request to continue.") : /*#__PURE__*/React.createElement(CB, {
    value: text,
    onChange: setText,
    showInternalToggle: true,
    internal: internal,
    onToggleInternal: setInternal,
    onSend: () => {
      if (text.trim()) {
        onSend(text, internal);
        setText('');
      }
    }
  })));
}
window.InboxMain = {
  ConvList,
  MemberPanel,
  ConvView
};
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/inbox/Inbox.jsx", error: String((e && e.message) || e) }); }

// ui_kits/inbox/Shell.jsx
try { (() => {
/* Sild Support Inbox — interactive UI-kit recreation (spec §8).
   Composes Sild DS components (window.Sild) + icons (window.I) + data (window.SILD_DATA). */
const {
  useState
} = React;
const S = window.Sild,
  I = window.I,
  DATA = window.SILD_DATA;
const {
  Button,
  IconButton,
  Avatar,
  Badge,
  Tag,
  Input,
  Select,
  Switch,
  Card,
  Banner,
  Tooltip,
  Dialog,
  StatusPill,
  MessageBubble,
  ConversationRow,
  ComposerBar
} = S;
const LOGO = window.__resources && window.__resources.sildLogo || '../../assets/sild-logo.svg';
const MARK = window.__resources && window.__resources.sildMark || '../../assets/sild-mark-tile.svg';

/* ---------------- Login ---------------- */
function Login({
  onSignIn
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'var(--surface-page)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 380,
      background: 'var(--surface-card)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-xl)',
      boxShadow: 'var(--shadow-lg)',
      padding: 36,
      textAlign: 'center'
    }
  }, /*#__PURE__*/React.createElement("img", {
    src: MARK,
    width: "46",
    alt: "Sild",
    style: {
      borderRadius: 13
    }
  }), /*#__PURE__*/React.createElement("h1", {
    style: {
      fontSize: 24,
      marginTop: 18,
      letterSpacing: '-0.02em'
    }
  }, "Sild support inbox"), /*#__PURE__*/React.createElement("p", {
    style: {
      fontSize: 14,
      color: 'var(--text-secondary)',
      marginTop: 8,
      lineHeight: 1.5
    }
  }, "Sign in to the assignment queue. Agents see only the conversations they're assigned."), /*#__PURE__*/React.createElement("button", {
    onClick: onSignIn,
    style: {
      marginTop: 24,
      width: '100%',
      height: 46,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      gap: 10,
      border: '1px solid var(--border-default)',
      background: 'var(--white)',
      borderRadius: 'var(--radius-md)',
      fontFamily: 'var(--font-sans)',
      fontSize: 15,
      fontWeight: 600,
      color: 'var(--text-primary)',
      cursor: 'pointer'
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "18",
    height: "18",
    viewBox: "0 0 24 24"
  }, /*#__PURE__*/React.createElement("path", {
    fill: "#4285F4",
    d: "M22.5 12.2c0-.7-.1-1.4-.2-2H12v3.9h5.9a5 5 0 0 1-2.2 3.3v2.7h3.6c2.1-1.9 3.2-4.8 3.2-7.9z"
  }), /*#__PURE__*/React.createElement("path", {
    fill: "#34A853",
    d: "M12 23c2.9 0 5.4-1 7.2-2.6l-3.6-2.7c-1 .7-2.3 1.1-3.6 1.1-2.8 0-5.1-1.9-6-4.4H2.3v2.8A11 11 0 0 0 12 23z"
  }), /*#__PURE__*/React.createElement("path", {
    fill: "#FBBC05",
    d: "M6 14.4a6.6 6.6 0 0 1 0-4.2V7.4H2.3a11 11 0 0 0 0 9.8z"
  }), /*#__PURE__*/React.createElement("path", {
    fill: "#EA4335",
    d: "M12 5.4c1.6 0 3 .5 4.1 1.6l3.1-3.1A11 11 0 0 0 2.3 7.4l3.7 2.8c.9-2.6 3.2-4.8 6-4.8z"
  })), "Continue with Google"), /*#__PURE__*/React.createElement("p", {
    style: {
      fontSize: 12,
      color: 'var(--text-tertiary)',
      marginTop: 18
    }
  }, "Admin identity is separate from chat end-users.")));
}

/* ---------------- Nav rail ---------------- */
function NavRail({
  view,
  setView,
  onSignOut
}) {
  const item = (key, label, icon) => /*#__PURE__*/React.createElement(Tooltip, {
    content: label,
    side: "right"
  }, /*#__PURE__*/React.createElement("button", {
    onClick: () => setView(key),
    "aria-label": label,
    style: {
      width: 44,
      height: 44,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      borderRadius: 'var(--radius-md)',
      border: 0,
      cursor: 'pointer',
      color: view === key ? 'var(--brand)' : 'var(--slate-400)',
      background: view === key ? 'var(--brand-subtle)' : 'transparent'
    }
  }, icon({
    width: 22,
    height: 22
  })));
  return /*#__PURE__*/React.createElement("div", {
    style: {
      width: 64,
      flex: 'none',
      background: 'var(--surface-card)',
      borderRight: '1px solid var(--border-default)',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      padding: '14px 0',
      gap: 6
    }
  }, /*#__PURE__*/React.createElement("img", {
    src: MARK,
    width: "34",
    alt: "Sild",
    style: {
      borderRadius: 10,
      marginBottom: 8
    }
  }), item('inbox', 'Inbox', I.Inbox), item('settings', 'Settings', I.Settings), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1
    }
  }), /*#__PURE__*/React.createElement(Tooltip, {
    content: "Sign out",
    side: "right"
  }, /*#__PURE__*/React.createElement("button", {
    onClick: onSignOut,
    "aria-label": "Account",
    style: {
      border: 0,
      background: 'transparent',
      cursor: 'pointer',
      padding: 0,
      borderRadius: '50%'
    }
  }, /*#__PURE__*/React.createElement(Avatar, {
    name: DATA.agent.name,
    size: 36
  }))));
}
window.InboxParts = {
  Login,
  NavRail
};
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/inbox/Shell.jsx", error: String((e && e.message) || e) }); }

// ui_kits/inbox/data.js
try { (() => {
/* Fake inbox data for the Sild support-inbox UI kit. */
window.SILD_DATA = function () {
  const agent = {
    name: 'Liis Mägi',
    email: 'liis@sild.io',
    role: 'agent'
  };
  const conversations = [{
    id: 'c_8842',
    name: 'Mari Tamm',
    presence: 'online',
    channel: 'app',
    reference: 'trip_8842',
    status: 'assigned',
    unread: 0,
    time: '2m',
    preview: "My driver still hasn't arrived",
    members: [{
      name: 'Mari Tamm',
      role: 'client',
      meta: {
        phone: '+372 5123 4567',
        app_version: '2.3.1',
        role: 'client'
      }
    }, {
      name: 'Driver 9',
      role: 'driver',
      meta: {
        phone: '+372 5987 6543',
        app_version: '2.3.0',
        role: 'driver'
      }
    }],
    messages: [{
      id: 1,
      dir: 'in',
      author: 'Mari Tamm',
      time: '2:01 PM',
      body: "Hi — my driver still hasn't arrived and the app says they're 2 min away for the last 10 minutes."
    }, {
      id: 2,
      dir: 'out',
      author: 'You',
      time: '2:03 PM',
      body: "Hi Mari, sorry about that. Let me check with the driver right now.",
      read: 'Read 2:04 PM'
    }, {
      id: 3,
      internal: true,
      author: 'You',
      time: '2:03 PM',
      body: "VIP rider — escalate to dispatch if not moving in 5 min."
    }, {
      id: 4,
      dir: 'in',
      author: 'Mari Tamm',
      time: '2:05 PM',
      body: "Thank you! I have a flight to catch."
    }]
  }, {
    id: 'c_5512',
    name: 'support@acme.com',
    presence: 'offline',
    channel: 'email',
    reference: 'order_5512',
    status: 'queued',
    unread: 2,
    time: '14m',
    preview: 'Re: refund for order 5512 — still not received',
    members: [{
      name: 'support@acme.com',
      role: 'client',
      meta: {
        email: 'support@acme.com',
        role: 'email contact'
      }
    }],
    messages: [{
      id: 1,
      dir: 'in',
      author: 'support@acme.com',
      channel: 'email',
      time: '1:40 PM',
      body: "Following up — the refund for order 5512 still hasn't landed. It's been 6 business days."
    }]
  }, {
    id: 'c_7731',
    name: 'Jaan Kask',
    presence: 'away',
    channel: 'app',
    reference: 'trip_7731',
    status: 'assigned',
    unread: 0,
    time: '1h',
    preview: 'Thanks, that worked!',
    members: [{
      name: 'Jaan Kask',
      role: 'client',
      meta: {
        phone: '+372 5444 1212',
        app_version: '2.2.9',
        role: 'client'
      }
    }],
    messages: [{
      id: 1,
      dir: 'in',
      author: 'Jaan Kask',
      time: '12:50 PM',
      body: "I can't add a payment card — it keeps failing."
    }, {
      id: 2,
      dir: 'out',
      author: 'You',
      time: '12:54 PM',
      body: "Try removing the old card first, then re-adding. There was a stale token on your account.",
      read: 'Read 12:55 PM'
    }, {
      id: 3,
      dir: 'in',
      author: 'Jaan Kask',
      time: '1:02 PM',
      body: 'Thanks, that worked!'
    }]
  }, {
    id: 'c_3300',
    name: 'Guest · web',
    presence: 'online',
    channel: 'app',
    reference: 'guest_7f3a',
    status: 'queued',
    unread: 1,
    time: '3h',
    preview: 'How do I change my pickup address?',
    members: [{
      name: 'Guest · web',
      role: 'client',
      meta: {
        guest: 'true',
        app_version: 'web 1.0'
      }
    }],
    messages: [{
      id: 1,
      dir: 'in',
      author: 'Guest',
      time: '11:20 AM',
      body: 'How do I change my pickup address after booking?'
    }]
  }, {
    id: 'c_2014',
    name: 'Pille Saar',
    presence: 'offline',
    channel: 'app',
    reference: 'trip_2014',
    status: 'closed',
    unread: 0,
    time: 'Yesterday',
    preview: 'Conversation closed',
    members: [{
      name: 'Pille Saar',
      role: 'client',
      meta: {
        phone: '+372 5333 9090',
        role: 'client'
      }
    }],
    messages: [{
      id: 1,
      dir: 'in',
      author: 'Pille Saar',
      time: 'Yesterday',
      body: 'Driver was great, thank you!'
    }, {
      id: 2,
      system: true,
      body: 'Conversation closed by Liis Mägi'
    }]
  }];
  return {
    agent,
    conversations
  };
}();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/inbox/data.js", error: String((e && e.message) || e) }); }

// ui_kits/sild-bundle.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/* Sild local component bundle — generated from components/ sources for self-contained UI kits.
   (Mirrors window.SildDesignSystem_a95234 from _ds_bundle.js; kept local so kits render offline.) */
window.Sild = window.Sild || {};
(function () {
  const React = window.React;

  // ---- Spinner ----
  window.Sild.Spinner = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-spinner{display:inline-block;border-radius:50%;border-style:solid;border-color:var(--border-strong);
  border-right-color:var(--brand);animation:sild-spin .6s linear infinite}
@keyframes sild-spin{to{transform:rotate(360deg)}}
@media (prefers-reduced-motion:reduce){.sild-spinner{animation-duration:1.2s}}
`;
      document.head.appendChild(s);
    }
    const SIZES = {
      sm: 16,
      md: 22,
      lg: 32
    };
    function Spinner({
      size = 'md',
      className = '',
      style = {},
      ...rest
    }) {
      injectCss();
      const px = typeof size === 'number' ? size : SIZES[size] || 22;
      const bw = Math.max(2, Math.round(px / 9));
      return /*#__PURE__*/React.createElement("span", _extends({
        className: ['sild-spinner', className].filter(Boolean).join(' '),
        role: "status",
        "aria-label": "Loading",
        style: {
          width: px,
          height: px,
          borderWidth: bw,
          ...style
        }
      }, rest));
    }
    return Spinner;
  }();

  // ---- Avatar ----
  window.Sild.Avatar = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-avatar{position:relative;display:inline-flex;align-items:center;justify-content:center;
  border-radius:var(--radius-full);font-family:var(--font-sans);font-weight:600;color:#fff;
  background:var(--brand);overflow:visible;flex:none;user-select:none}
.sild-avatar--square{border-radius:var(--radius-md)}
.sild-avatar__img{width:100%;height:100%;object-fit:cover;border-radius:inherit}
.sild-avatar__presence{position:absolute;right:-1px;bottom:-1px;border-radius:50%;
  border:2px solid var(--surface-card);box-sizing:content-box}
.sild-avatar__presence--online{background:var(--status-online)}
.sild-avatar__presence--away{background:var(--status-queued)}
.sild-avatar__presence--offline{background:var(--slate-400)}
`;
      document.head.appendChild(s);
    }
    const SIZES = {
      xs: 22,
      sm: 28,
      md: 36,
      lg: 44,
      xl: 56
    };
    const PALETTE = ['#3D63FF', '#FF7A45', '#18A957', '#7C5CFF', '#0EA5A5', '#E0599B', '#D9881A', '#2440B8'];
    function initials(name = '') {
      const parts = name.trim().split(/\s+/).filter(Boolean);
      if (!parts.length) return '?';
      if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    function colorFor(name = '') {
      let h = 0;
      for (let i = 0; i < name.length; i++) h = h * 31 + name.charCodeAt(i) >>> 0;
      return PALETTE[h % PALETTE.length];
    }
    function Avatar({
      name = '',
      src = null,
      size = 'md',
      shape = 'circle',
      presence = null,
      className = '',
      style = {},
      ...rest
    }) {
      injectCss();
      const px = typeof size === 'number' ? size : SIZES[size] || 36;
      const cls = ['sild-avatar', shape === 'square' ? 'sild-avatar--square' : '', className].filter(Boolean).join(' ');
      const dot = Math.max(8, Math.round(px * 0.28));
      return /*#__PURE__*/React.createElement("span", _extends({
        className: cls,
        style: {
          width: px,
          height: px,
          fontSize: Math.round(px * 0.38),
          background: src ? 'var(--surface-sunken)' : colorFor(name),
          ...style
        },
        title: name || undefined
      }, rest), src ? /*#__PURE__*/React.createElement("img", {
        className: "sild-avatar__img",
        src: src,
        alt: name
      }) : initials(name), presence && /*#__PURE__*/React.createElement("span", {
        className: `sild-avatar__presence sild-avatar__presence--${presence}`,
        style: {
          width: dot,
          height: dot
        }
      }));
    }
    return Avatar;
  }();

  // ---- AvatarStack ----
  window.Sild.AvatarStack = function () {
    function AvatarStack({
      people = [],
      max = 4,
      size = 'sm',
      className = '',
      ...rest
    }) {
      const shown = people.slice(0, max);
      const overflow = people.length - shown.length;
      const px = typeof size === 'number' ? size : {
        xs: 22,
        sm: 28,
        md: 36,
        lg: 44,
        xl: 56
      }[size] || 28;
      const overlap = Math.round(px * 0.32);
      return /*#__PURE__*/React.createElement("span", _extends({
        className: className,
        style: {
          display: 'inline-flex',
          alignItems: 'center'
        }
      }, rest), shown.map((p, i) => /*#__PURE__*/React.createElement("span", {
        key: i,
        style: {
          marginLeft: i === 0 ? 0 : -overlap,
          borderRadius: '50%',
          boxShadow: '0 0 0 2px var(--surface-card)'
        }
      }, /*#__PURE__*/React.createElement(Avatar, {
        name: p.name,
        src: p.src,
        size: size
      }))), overflow > 0 && /*#__PURE__*/React.createElement("span", {
        style: {
          marginLeft: -overlap,
          width: px,
          height: px,
          borderRadius: '50%',
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'var(--slate-100)',
          color: 'var(--text-secondary)',
          fontFamily: 'var(--font-sans)',
          fontWeight: 600,
          fontSize: Math.round(px * 0.34),
          boxShadow: '0 0 0 2px var(--surface-card)'
        }
      }, "+", overflow));
    }
    return AvatarStack;
  }();

  // ---- IconButton ----
  window.Sild.IconButton = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-iconbtn{display:inline-flex;align-items:center;justify-content:center;border-radius:var(--radius-md);
  border:1px solid transparent;background:transparent;color:var(--text-secondary);cursor:pointer;
  transition:background var(--duration-fast) var(--ease-standard),color var(--duration-fast) var(--ease-standard);}
.sild-iconbtn:hover{background:var(--surface-hover);color:var(--text-primary)}
.sild-iconbtn:active{background:var(--surface-active)}
.sild-iconbtn:focus-visible{outline:none;box-shadow:var(--ring)}
.sild-iconbtn[disabled]{cursor:not-allowed;opacity:.45}
.sild-iconbtn--sm{width:30px;height:30px}
.sild-iconbtn--md{width:36px;height:36px}
.sild-iconbtn--lg{width:44px;height:44px}
.sild-iconbtn--solid{background:var(--brand);color:#fff}
.sild-iconbtn--solid:hover{background:var(--brand-hover);color:#fff}
.sild-iconbtn--solid:active{background:var(--brand-active)}
.sild-iconbtn--bordered{border-color:var(--border-default);background:var(--white)}
@media (prefers-reduced-motion:reduce){.sild-iconbtn{transition:none}}
`;
      document.head.appendChild(s);
    }
    function IconButton({
      size = 'md',
      variant = 'ghost',
      disabled = false,
      className = '',
      'aria-label': ariaLabel,
      children,
      ...rest
    }) {
      injectCss();
      const cls = ['sild-iconbtn', `sild-iconbtn--${size}`, variant !== 'ghost' ? `sild-iconbtn--${variant}` : '', className].filter(Boolean).join(' ');
      return /*#__PURE__*/React.createElement("button", _extends({
        type: "button",
        className: cls,
        disabled: disabled,
        "aria-label": ariaLabel
      }, rest), children);
    }
    return IconButton;
  }();

  // ---- Button ----
  window.Sild.Button = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-btn{--_bg:var(--brand);--_bg-h:var(--brand-hover);--_bg-a:var(--brand-active);--_fg:#fff;--_bd:transparent;
  display:inline-flex;align-items:center;justify-content:center;gap:8px;font-family:var(--font-sans);
  font-weight:600;letter-spacing:-.01em;border:1px solid var(--_bd);border-radius:var(--radius-md);
  background:var(--_bg);color:var(--_fg);cursor:pointer;white-space:nowrap;
  transition:background var(--duration-fast) var(--ease-standard),box-shadow var(--duration-fast) var(--ease-standard),transform var(--duration-instant) var(--ease-standard);}
.sild-btn:hover{background:var(--_bg-h)}
.sild-btn:active{background:var(--_bg-a);transform:translateY(1px)}
.sild-btn:focus-visible{outline:none;box-shadow:var(--ring)}
.sild-btn[disabled]{cursor:not-allowed;opacity:.5}
.sild-btn[disabled]:active{transform:none}
.sild-btn--md{height:40px;padding:0 16px;font-size:14px}
.sild-btn--sm{height:32px;padding:0 12px;font-size:13px;border-radius:var(--radius-sm)}
.sild-btn--lg{height:48px;padding:0 22px;font-size:16px}
.sild-btn--full{width:100%}
.sild-btn--secondary{--_bg:var(--white);--_bg-h:var(--surface-hover);--_bg-a:var(--surface-active);--_fg:var(--text-primary);--_bd:var(--border-default)}
.sild-btn--ghost{--_bg:transparent;--_bg-h:var(--surface-hover);--_bg-a:var(--surface-active);--_fg:var(--text-primary);--_bd:transparent}
.sild-btn--danger{--_bg:var(--danger);--_bg-h:var(--danger-hover);--_bg-a:var(--red-600);--_fg:#fff}
.sild-btn--danger:focus-visible{box-shadow:var(--ring-danger)}
.sild-btn__spin{width:15px;height:15px;border-radius:50%;border:2px solid currentColor;border-right-color:transparent;animation:sild-btn-spin .6s linear infinite}
@keyframes sild-btn-spin{to{transform:rotate(360deg)}}
@media (prefers-reduced-motion:reduce){.sild-btn{transition:none}.sild-btn__spin{animation-duration:1.2s}}
`;
      document.head.appendChild(s);
    }
    function Button({
      variant = 'primary',
      size = 'md',
      disabled = false,
      loading = false,
      fullWidth = false,
      iconLeft = null,
      iconRight = null,
      type = 'button',
      className = '',
      children,
      ...rest
    }) {
      injectCss();
      const cls = ['sild-btn', `sild-btn--${size}`, variant !== 'primary' ? `sild-btn--${variant}` : '', fullWidth ? 'sild-btn--full' : '', className].filter(Boolean).join(' ');
      return /*#__PURE__*/React.createElement("button", _extends({
        type: type,
        className: cls,
        disabled: disabled || loading
      }, rest), loading && /*#__PURE__*/React.createElement("span", {
        className: "sild-btn__spin",
        "aria-hidden": "true"
      }), !loading && iconLeft, children && /*#__PURE__*/React.createElement("span", null, children), !loading && iconRight);
    }
    return Button;
  }();

  // ---- Badge ----
  window.Sild.Badge = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-badge{display:inline-flex;align-items:center;gap:5px;font-family:var(--font-sans);font-weight:600;
  font-size:12px;line-height:1;border-radius:var(--radius-full);padding:4px 9px;white-space:nowrap;
  --_bg:var(--surface-sunken);--_fg:var(--text-secondary);background:var(--_bg);color:var(--_fg)}
.sild-badge--neutral{--_bg:var(--slate-100);--_fg:var(--slate-700)}
.sild-badge--brand{--_bg:var(--blue-50);--_fg:var(--blue-700)}
.sild-badge--success{--_bg:var(--success-subtle);--_fg:var(--green-600)}
.sild-badge--warning{--_bg:var(--warning-subtle);--_fg:var(--amber-600)}
.sild-badge--danger{--_bg:var(--danger-subtle);--_fg:var(--red-600)}
.sild-badge--accent{--_bg:var(--accent-subtle);--_fg:var(--coral-600)}
.sild-badge--solid{--_bg:var(--brand);--_fg:#fff}
.sild-badge__dot{width:6px;height:6px;border-radius:50%;background:currentColor}
.sild-badge--count{min-width:18px;height:18px;padding:0 5px;justify-content:center;--_bg:var(--accent);--_fg:#fff}
`;
      document.head.appendChild(s);
    }
    function Badge({
      variant = 'neutral',
      dot = false,
      count = false,
      className = '',
      children,
      ...rest
    }) {
      injectCss();
      const cls = ['sild-badge', `sild-badge--${variant}`, count ? 'sild-badge--count' : '', className].filter(Boolean).join(' ');
      return /*#__PURE__*/React.createElement("span", _extends({
        className: cls
      }, rest), dot && /*#__PURE__*/React.createElement("span", {
        className: "sild-badge__dot",
        "aria-hidden": "true"
      }), children);
    }
    return Badge;
  }();

  // ---- Tag ----
  window.Sild.Tag = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-tag{display:inline-flex;align-items:center;gap:6px;font-family:var(--font-sans);font-size:13px;
  font-weight:500;color:var(--text-secondary);background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-sm);padding:3px 8px;line-height:1.4;white-space:nowrap}
.sild-tag--mono{font-family:var(--font-mono);font-size:12px;background:var(--surface-sunken);border-color:var(--border-subtle)}
.sild-tag__remove{display:inline-flex;cursor:pointer;color:var(--text-tertiary);border:0;background:none;padding:0;
  border-radius:var(--radius-xs);transition:color var(--duration-fast)}
.sild-tag__remove:hover{color:var(--text-primary)}
`;
      document.head.appendChild(s);
    }
    function Tag({
      mono = false,
      onRemove,
      className = '',
      children,
      ...rest
    }) {
      injectCss();
      const cls = ['sild-tag', mono ? 'sild-tag--mono' : '', className].filter(Boolean).join(' ');
      return /*#__PURE__*/React.createElement("span", _extends({
        className: cls
      }, rest), children, onRemove && /*#__PURE__*/React.createElement("button", {
        type: "button",
        className: "sild-tag__remove",
        "aria-label": "Remove",
        onClick: onRemove
      }, /*#__PURE__*/React.createElement("svg", {
        width: "12",
        height: "12",
        viewBox: "0 0 12 12",
        fill: "none"
      }, /*#__PURE__*/React.createElement("path", {
        d: "M3 3l6 6M9 3l-6 6",
        stroke: "currentColor",
        strokeWidth: "1.6",
        strokeLinecap: "round"
      }))));
    }
    return Tag;
  }();

  // ---- Input ----
  window.Sild.Input = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-field{display:flex;flex-direction:column;gap:6px;font-family:var(--font-sans)}
.sild-field__label{font-size:13px;font-weight:600;color:var(--text-primary)}
.sild-field__req{color:var(--danger);margin-left:2px}
.sild-field__hint{font-size:12px;color:var(--text-tertiary)}
.sild-field__hint--error{color:var(--danger)}
.sild-input{display:flex;align-items:center;gap:8px;background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-md);transition:border-color var(--duration-fast),box-shadow var(--duration-fast);padding:0 12px}
.sild-input:focus-within{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-input--error{border-color:var(--danger)}
.sild-input--error:focus-within{box-shadow:var(--ring-danger)}
.sild-input--disabled{background:var(--surface-sunken);opacity:.7;cursor:not-allowed}
.sild-input--sm{height:34px}
.sild-input--md{height:40px}
.sild-input--lg{height:46px}
.sild-input__el{flex:1;border:0;outline:none;background:transparent;font-family:inherit;font-size:14px;
  color:var(--text-primary);min-width:0;height:100%}
.sild-input__el::placeholder{color:var(--text-tertiary)}
.sild-input__icon{display:inline-flex;color:var(--text-tertiary);flex:none}
`;
      document.head.appendChild(s);
    }
    function Input({
      label,
      hint,
      error,
      required = false,
      size = 'md',
      iconLeft = null,
      iconRight = null,
      disabled = false,
      className = '',
      id,
      ...rest
    }) {
      injectCss();
      const fid = id || (label ? 'in-' + label.replace(/\s+/g, '-').toLowerCase() : undefined);
      const wrapCls = ['sild-input', `sild-input--${size}`, error ? 'sild-input--error' : '', disabled ? 'sild-input--disabled' : ''].filter(Boolean).join(' ');
      return /*#__PURE__*/React.createElement("div", {
        className: ['sild-field', className].filter(Boolean).join(' ')
      }, label && /*#__PURE__*/React.createElement("label", {
        className: "sild-field__label",
        htmlFor: fid
      }, label, required && /*#__PURE__*/React.createElement("span", {
        className: "sild-field__req"
      }, "*")), /*#__PURE__*/React.createElement("div", {
        className: wrapCls
      }, iconLeft && /*#__PURE__*/React.createElement("span", {
        className: "sild-input__icon"
      }, iconLeft), /*#__PURE__*/React.createElement("input", _extends({
        id: fid,
        className: "sild-input__el",
        disabled: disabled,
        "aria-invalid": !!error
      }, rest)), iconRight && /*#__PURE__*/React.createElement("span", {
        className: "sild-input__icon"
      }, iconRight)), (error || hint) && /*#__PURE__*/React.createElement("span", {
        className: ['sild-field__hint', error ? 'sild-field__hint--error' : ''].filter(Boolean).join(' ')
      }, error || hint));
    }
    return Input;
  }();

  // ---- Textarea ----
  window.Sild.Textarea = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-textarea{width:100%;font-family:var(--font-sans);font-size:14px;color:var(--text-primary);
  background:var(--white);border:1px solid var(--border-default);border-radius:var(--radius-md);
  padding:10px 12px;resize:vertical;line-height:1.5;transition:border-color var(--duration-fast),box-shadow var(--duration-fast);outline:none}
.sild-textarea::placeholder{color:var(--text-tertiary)}
.sild-textarea:focus{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-textarea--error{border-color:var(--danger)}
.sild-textarea--error:focus{box-shadow:var(--ring-danger)}
.sild-textarea[disabled]{background:var(--surface-sunken);opacity:.7}
`;
      document.head.appendChild(s);
    }
    function Textarea({
      label,
      hint,
      error,
      required = false,
      rows = 4,
      className = '',
      id,
      ...rest
    }) {
      injectCss();
      const fid = id || (label ? 'ta-' + label.replace(/\s+/g, '-').toLowerCase() : undefined);
      return /*#__PURE__*/React.createElement("div", {
        className: ['sild-field', className].filter(Boolean).join(' ')
      }, label && /*#__PURE__*/React.createElement("label", {
        className: "sild-field__label",
        htmlFor: fid
      }, label, required && /*#__PURE__*/React.createElement("span", {
        className: "sild-field__req"
      }, "*")), /*#__PURE__*/React.createElement("textarea", _extends({
        id: fid,
        rows: rows,
        className: ['sild-textarea', error ? 'sild-textarea--error' : ''].filter(Boolean).join(' '),
        "aria-invalid": !!error
      }, rest)), (error || hint) && /*#__PURE__*/React.createElement("span", {
        className: ['sild-field__hint', error ? 'sild-field__hint--error' : ''].filter(Boolean).join(' ')
      }, error || hint));
    }
    return Textarea;
  }();

  // ---- Select ----
  window.Sild.Select = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-select{position:relative;display:flex;align-items:center;background:var(--white);
  border:1px solid var(--border-default);border-radius:var(--radius-md);transition:border-color var(--duration-fast),box-shadow var(--duration-fast)}
.sild-select:focus-within{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-select--sm{height:34px}.sild-select--md{height:40px}.sild-select--lg{height:46px}
.sild-select__el{appearance:none;-webkit-appearance:none;border:0;outline:none;background:transparent;
  font-family:var(--font-sans);font-size:14px;color:var(--text-primary);height:100%;width:100%;
  padding:0 36px 0 12px;cursor:pointer}
.sild-select__el[disabled]{cursor:not-allowed;color:var(--text-disabled)}
.sild-select__chev{position:absolute;right:12px;pointer-events:none;color:var(--text-tertiary)}
`;
      document.head.appendChild(s);
    }
    function Select({
      label,
      hint,
      error,
      required = false,
      size = 'md',
      options = [],
      placeholder,
      className = '',
      id,
      children,
      ...rest
    }) {
      injectCss();
      const fid = id || (label ? 'sel-' + label.replace(/\s+/g, '-').toLowerCase() : undefined);
      return /*#__PURE__*/React.createElement("div", {
        className: ['sild-field', className].filter(Boolean).join(' ')
      }, label && /*#__PURE__*/React.createElement("label", {
        className: "sild-field__label",
        htmlFor: fid
      }, label, required && /*#__PURE__*/React.createElement("span", {
        className: "sild-field__req"
      }, "*")), /*#__PURE__*/React.createElement("div", {
        className: ['sild-select', `sild-select--${size}`].filter(Boolean).join(' '),
        style: error ? {
          borderColor: 'var(--danger)'
        } : undefined
      }, /*#__PURE__*/React.createElement("select", _extends({
        id: fid,
        className: "sild-select__el"
      }, rest), placeholder && /*#__PURE__*/React.createElement("option", {
        value: "",
        disabled: true
      }, placeholder), options.map(o => {
        const val = typeof o === 'string' ? o : o.value;
        const lab = typeof o === 'string' ? o : o.label;
        return /*#__PURE__*/React.createElement("option", {
          key: val,
          value: val
        }, lab);
      }), children), /*#__PURE__*/React.createElement("span", {
        className: "sild-select__chev"
      }, /*#__PURE__*/React.createElement("svg", {
        width: "16",
        height: "16",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "2",
        strokeLinecap: "round",
        strokeLinejoin: "round"
      }, /*#__PURE__*/React.createElement("path", {
        d: "M6 9l6 6 6-6"
      })))), (error || hint) && /*#__PURE__*/React.createElement("span", {
        className: ['sild-field__hint', error ? 'sild-field__hint--error' : ''].filter(Boolean).join(' ')
      }, error || hint));
    }
    return Select;
  }();

  // ---- Checkbox ----
  window.Sild.Checkbox = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-check{display:inline-flex;align-items:flex-start;gap:9px;font-family:var(--font-sans);cursor:pointer;user-select:none}
.sild-check[aria-disabled="true"]{cursor:not-allowed;opacity:.55}
.sild-check__box{flex:none;width:18px;height:18px;border-radius:var(--radius-xs);border:1.5px solid var(--border-strong);
  background:var(--white);display:inline-flex;align-items:center;justify-content:center;color:#fff;margin-top:1px;
  transition:background var(--duration-fast),border-color var(--duration-fast)}
.sild-check input:focus-visible + .sild-check__box{box-shadow:var(--ring);border-color:var(--border-focus)}
.sild-check__box--on{background:var(--brand);border-color:var(--brand)}
.sild-check__txt{font-size:14px;color:var(--text-primary);line-height:1.35}
.sild-check__sub{font-size:12px;color:var(--text-tertiary);margin-top:1px}
.sild-check__hide{position:absolute;opacity:0;width:0;height:0;pointer-events:none}
`;
      document.head.appendChild(s);
    }
    function Checkbox({
      checked = false,
      onChange,
      label,
      description,
      disabled = false,
      className = '',
      ...rest
    }) {
      injectCss();
      return /*#__PURE__*/React.createElement("label", {
        className: ['sild-check', className].filter(Boolean).join(' '),
        "aria-disabled": disabled
      }, /*#__PURE__*/React.createElement("input", _extends({
        type: "checkbox",
        className: "sild-check__hide",
        checked: checked,
        disabled: disabled,
        onChange: e => onChange && onChange(e.target.checked, e)
      }, rest)), /*#__PURE__*/React.createElement("span", {
        className: ['sild-check__box', checked ? 'sild-check__box--on' : ''].filter(Boolean).join(' ')
      }, checked && /*#__PURE__*/React.createElement("svg", {
        width: "12",
        height: "12",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "3.2",
        strokeLinecap: "round",
        strokeLinejoin: "round"
      }, /*#__PURE__*/React.createElement("path", {
        d: "M20 6 9 17l-5-5"
      }))), (label || description) && /*#__PURE__*/React.createElement("span", null, label && /*#__PURE__*/React.createElement("span", {
        className: "sild-check__txt"
      }, label), description && /*#__PURE__*/React.createElement("span", {
        className: "sild-check__sub"
      }, description)));
    }
    return Checkbox;
  }();

  // ---- Switch ----
  window.Sild.Switch = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-switch{display:inline-flex;align-items:center;gap:10px;font-family:var(--font-sans);cursor:pointer;user-select:none}
.sild-switch[aria-disabled="true"]{cursor:not-allowed;opacity:.55}
.sild-switch__track{position:relative;flex:none;width:36px;height:20px;border-radius:var(--radius-full);
  background:var(--slate-300);transition:background var(--duration-base) var(--ease-standard)}
.sild-switch__track--on{background:var(--brand)}
.sild-switch__knob{position:absolute;top:2px;left:2px;width:16px;height:16px;border-radius:50%;background:#fff;
  box-shadow:var(--shadow-xs);transition:transform var(--duration-base) var(--ease-standard)}
.sild-switch__track--on .sild-switch__knob{transform:translateX(16px)}
.sild-switch input:focus-visible + .sild-switch__track{box-shadow:var(--ring)}
.sild-switch__txt{font-size:14px;color:var(--text-primary)}
.sild-switch__hide{position:absolute;opacity:0;width:0;height:0}
@media (prefers-reduced-motion:reduce){.sild-switch__track,.sild-switch__knob{transition:none}}
`;
      document.head.appendChild(s);
    }
    function Switch({
      checked = false,
      onChange,
      label,
      disabled = false,
      className = '',
      ...rest
    }) {
      injectCss();
      return /*#__PURE__*/React.createElement("label", {
        className: ['sild-switch', className].filter(Boolean).join(' '),
        "aria-disabled": disabled
      }, /*#__PURE__*/React.createElement("input", _extends({
        type: "checkbox",
        role: "switch",
        className: "sild-switch__hide",
        checked: checked,
        disabled: disabled,
        onChange: e => onChange && onChange(e.target.checked, e)
      }, rest)), /*#__PURE__*/React.createElement("span", {
        className: ['sild-switch__track', checked ? 'sild-switch__track--on' : ''].filter(Boolean).join(' ')
      }, /*#__PURE__*/React.createElement("span", {
        className: "sild-switch__knob"
      })), label && /*#__PURE__*/React.createElement("span", {
        className: "sild-switch__txt"
      }, label));
    }
    return Switch;
  }();

  // ---- Card ----
  window.Sild.Card = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-card{background:var(--surface-card);border:1px solid var(--border-default);border-radius:var(--radius-lg);
  box-shadow:var(--shadow-sm);overflow:hidden;font-family:var(--font-sans)}
.sild-card--flat{box-shadow:none}
.sild-card--hover{transition:box-shadow var(--duration-base) var(--ease-standard),border-color var(--duration-base),transform var(--duration-base)}
.sild-card--hover:hover{box-shadow:var(--shadow-md);border-color:var(--border-strong)}
.sild-card__header{padding:16px 18px;border-bottom:1px solid var(--border-subtle);display:flex;align-items:center;justify-content:space-between;gap:12px}
.sild-card__title{font-size:15px;font-weight:700;color:var(--text-primary);letter-spacing:-.01em}
.sild-card__sub{font-size:13px;color:var(--text-tertiary);margin-top:2px}
.sild-card__body{padding:18px}
.sild-card__footer{padding:14px 18px;border-top:1px solid var(--border-subtle);background:var(--surface-page);display:flex;gap:10px;justify-content:flex-end}
`;
      document.head.appendChild(s);
    }
    function Card({
      title,
      subtitle,
      headerAction,
      footer,
      flat = false,
      hoverable = false,
      padded = true,
      className = '',
      children,
      ...rest
    }) {
      injectCss();
      const cls = ['sild-card', flat ? 'sild-card--flat' : '', hoverable ? 'sild-card--hover' : '', className].filter(Boolean).join(' ');
      return /*#__PURE__*/React.createElement("div", _extends({
        className: cls
      }, rest), (title || headerAction) && /*#__PURE__*/React.createElement("div", {
        className: "sild-card__header"
      }, /*#__PURE__*/React.createElement("div", null, title && /*#__PURE__*/React.createElement("div", {
        className: "sild-card__title"
      }, title), subtitle && /*#__PURE__*/React.createElement("div", {
        className: "sild-card__sub"
      }, subtitle)), headerAction), padded ? /*#__PURE__*/React.createElement("div", {
        className: "sild-card__body"
      }, children) : children, footer && /*#__PURE__*/React.createElement("div", {
        className: "sild-card__footer"
      }, footer));
    }
    return Card;
  }();

  // ---- Banner ----
  window.Sild.Banner = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-banner{display:flex;gap:11px;align-items:flex-start;font-family:var(--font-sans);
  border:1px solid var(--_bd,var(--border-default));background:var(--_bg,var(--surface-sunken));
  border-radius:var(--radius-md);padding:12px 14px;color:var(--text-primary)}
.sild-banner--info{--_bg:var(--blue-50);--_bd:var(--blue-200)}
.sild-banner--success{--_bg:var(--success-subtle);--_bd:#B6E6C9}
.sild-banner--warning{--_bg:var(--warning-subtle);--_bd:#F5DCA6}
.sild-banner--danger{--_bg:var(--danger-subtle);--_bd:#F4C2C3}
.sild-banner__icon{flex:none;margin-top:1px}
.sild-banner--info .sild-banner__icon{color:var(--blue-600)}
.sild-banner--success .sild-banner__icon{color:var(--green-600)}
.sild-banner--warning .sild-banner__icon{color:var(--amber-600)}
.sild-banner--danger .sild-banner__icon{color:var(--red-600)}
.sild-banner__body{flex:1;min-width:0}
.sild-banner__title{font-size:14px;font-weight:600;line-height:1.4}
.sild-banner__msg{font-size:13px;color:var(--text-secondary);line-height:1.5;margin-top:2px}
.sild-banner__close{flex:none;background:none;border:0;cursor:pointer;color:var(--text-tertiary);padding:2px;border-radius:var(--radius-xs)}
.sild-banner__close:hover{color:var(--text-primary);background:rgba(0,0,0,.05)}
`;
      document.head.appendChild(s);
    }
    const ICONS = {
      info: 'M12 16v-4M12 8h.01M12 22a10 10 0 100-20 10 10 0 000 20z',
      success: 'M22 11.08V12a10 10 0 11-5.93-9.14M22 4 12 14.01l-3-3',
      warning: 'M10.29 3.86 1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0zM12 9v4M12 17h.01',
      danger: 'M12 8v4M12 16h.01M12 22a10 10 0 100-20 10 10 0 000 20z'
    };
    function Banner({
      variant = 'info',
      title,
      onClose,
      className = '',
      children,
      ...rest
    }) {
      injectCss();
      return /*#__PURE__*/React.createElement("div", _extends({
        className: ['sild-banner', `sild-banner--${variant}`, className].filter(Boolean).join(' '),
        role: "status"
      }, rest), /*#__PURE__*/React.createElement("span", {
        className: "sild-banner__icon"
      }, /*#__PURE__*/React.createElement("svg", {
        width: "18",
        height: "18",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "2",
        strokeLinecap: "round",
        strokeLinejoin: "round"
      }, /*#__PURE__*/React.createElement("path", {
        d: ICONS[variant]
      }))), /*#__PURE__*/React.createElement("div", {
        className: "sild-banner__body"
      }, title && /*#__PURE__*/React.createElement("div", {
        className: "sild-banner__title"
      }, title), children && /*#__PURE__*/React.createElement("div", {
        className: "sild-banner__msg"
      }, children)), onClose && /*#__PURE__*/React.createElement("button", {
        type: "button",
        className: "sild-banner__close",
        "aria-label": "Dismiss",
        onClick: onClose
      }, /*#__PURE__*/React.createElement("svg", {
        width: "15",
        height: "15",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "2",
        strokeLinecap: "round"
      }, /*#__PURE__*/React.createElement("path", {
        d: "M18 6 6 18M6 6l12 12"
      }))));
    }
    return Banner;
  }();

  // ---- Tooltip ----
  window.Sild.Tooltip = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-tip-wrap{position:relative;display:inline-flex}
.sild-tip{position:absolute;z-index:50;background:var(--slate-900);color:#fff;font-family:var(--font-sans);
  font-size:12px;font-weight:500;line-height:1.4;padding:6px 9px;border-radius:var(--radius-sm);
  white-space:nowrap;box-shadow:var(--shadow-md);pointer-events:none;
  opacity:0;transform:translateY(2px);transition:opacity var(--duration-fast),transform var(--duration-fast)}
.sild-tip--show{opacity:1;transform:translateY(0)}
.sild-tip--top{bottom:calc(100% + 7px);left:50%;translate:-50% 0}
.sild-tip--bottom{top:calc(100% + 7px);left:50%;translate:-50% 0}
.sild-tip--left{right:calc(100% + 7px);top:50%;translate:0 -50%}
.sild-tip--right{left:calc(100% + 7px);top:50%;translate:0 -50%}
@media (prefers-reduced-motion:reduce){.sild-tip{transition:none}}
`;
      document.head.appendChild(s);
    }
    function Tooltip({
      content,
      side = 'top',
      className = '',
      children,
      ...rest
    }) {
      injectCss();
      const [show, setShow] = React.useState(false);
      return /*#__PURE__*/React.createElement("span", _extends({
        className: ['sild-tip-wrap', className].filter(Boolean).join(' '),
        onMouseEnter: () => setShow(true),
        onMouseLeave: () => setShow(false),
        onFocus: () => setShow(true),
        onBlur: () => setShow(false)
      }, rest), children, /*#__PURE__*/React.createElement("span", {
        role: "tooltip",
        className: ['sild-tip', `sild-tip--${side}`, show ? 'sild-tip--show' : ''].filter(Boolean).join(' ')
      }, content));
    }
    return Tooltip;
  }();

  // ---- Dialog ----
  window.Sild.Dialog = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-dialog__scrim{position:fixed;inset:0;background:var(--surface-overlay);z-index:1000;
  display:flex;align-items:center;justify-content:center;padding:24px;
  animation:sild-fade var(--duration-base) var(--ease-standard)}
.sild-dialog{background:var(--surface-card);border-radius:var(--radius-xl);box-shadow:var(--shadow-xl);
  width:100%;max-width:var(--_w,480px);max-height:calc(100vh - 48px);display:flex;flex-direction:column;
  font-family:var(--font-sans);overflow:hidden;animation:sild-pop var(--duration-base) var(--ease-out)}
.sild-dialog__head{padding:20px 22px 0;display:flex;align-items:flex-start;justify-content:space-between;gap:12px}
.sild-dialog__title{font-size:18px;font-weight:700;letter-spacing:-.01em;color:var(--text-primary)}
.sild-dialog__sub{font-size:13px;color:var(--text-tertiary);margin-top:3px}
.sild-dialog__body{padding:16px 22px;overflow-y:auto}
.sild-dialog__foot{padding:14px 22px 20px;display:flex;gap:10px;justify-content:flex-end}
.sild-dialog__x{background:none;border:0;cursor:pointer;color:var(--text-tertiary);padding:4px;border-radius:var(--radius-sm)}
.sild-dialog__x:hover{color:var(--text-primary);background:var(--surface-hover)}
@keyframes sild-fade{from{opacity:0}to{opacity:1}}
@keyframes sild-pop{from{opacity:0;transform:translateY(8px) scale(.98)}to{opacity:1;transform:none}}
@media (prefers-reduced-motion:reduce){.sild-dialog,.sild-dialog__scrim{animation:none}}
`;
      document.head.appendChild(s);
    }
    function Dialog({
      open = true,
      onClose,
      title,
      subtitle,
      footer,
      width = 480,
      className = '',
      children,
      ...rest
    }) {
      injectCss();
      if (!open) return null;
      return /*#__PURE__*/React.createElement("div", {
        className: "sild-dialog__scrim",
        onClick: e => {
          if (e.target === e.currentTarget && onClose) onClose();
        }
      }, /*#__PURE__*/React.createElement("div", _extends({
        className: ['sild-dialog', className].filter(Boolean).join(' '),
        role: "dialog",
        "aria-modal": "true",
        style: {
          '--_w': typeof width === 'number' ? width + 'px' : width
        }
      }, rest), (title || onClose) && /*#__PURE__*/React.createElement("div", {
        className: "sild-dialog__head"
      }, /*#__PURE__*/React.createElement("div", null, title && /*#__PURE__*/React.createElement("div", {
        className: "sild-dialog__title"
      }, title), subtitle && /*#__PURE__*/React.createElement("div", {
        className: "sild-dialog__sub"
      }, subtitle)), onClose && /*#__PURE__*/React.createElement("button", {
        type: "button",
        className: "sild-dialog__x",
        "aria-label": "Close",
        onClick: onClose
      }, /*#__PURE__*/React.createElement("svg", {
        width: "18",
        height: "18",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "2",
        strokeLinecap: "round"
      }, /*#__PURE__*/React.createElement("path", {
        d: "M18 6 6 18M6 6l12 12"
      })))), /*#__PURE__*/React.createElement("div", {
        className: "sild-dialog__body"
      }, children), footer && /*#__PURE__*/React.createElement("div", {
        className: "sild-dialog__foot"
      }, footer)));
    }
    return Dialog;
  }();

  // ---- StatusPill ----
  window.Sild.StatusPill = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-status{display:inline-flex;align-items:center;gap:6px;font-family:var(--font-sans);font-weight:600;
  font-size:12px;line-height:1;border-radius:var(--radius-full);padding:4px 9px 4px 8px;white-space:nowrap;
  background:var(--_bg);color:var(--_fg)}
.sild-status__dot{width:7px;height:7px;border-radius:50%;background:currentColor}
.sild-status--open{--_bg:var(--success-subtle);--_fg:var(--green-600)}
.sild-status--queued{--_bg:var(--warning-subtle);--_fg:var(--amber-600)}
.sild-status--assigned{--_bg:var(--blue-50);--_fg:var(--blue-700)}
.sild-status--closed{--_bg:var(--slate-100);--_fg:var(--slate-600)}
`;
      document.head.appendChild(s);
    }
    const LABELS = {
      open: 'Open',
      queued: 'Queued',
      assigned: 'Assigned',
      closed: 'Closed'
    };
    function StatusPill({
      status = 'open',
      label,
      className = '',
      ...rest
    }) {
      injectCss();
      return /*#__PURE__*/React.createElement("span", _extends({
        className: ['sild-status', `sild-status--${status}`, className].filter(Boolean).join(' ')
      }, rest), /*#__PURE__*/React.createElement("span", {
        className: "sild-status__dot",
        "aria-hidden": "true"
      }), label || LABELS[status] || status);
    }
    return StatusPill;
  }();

  // ---- MessageBubble ----
  window.Sild.MessageBubble = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-msg{display:flex;flex-direction:column;max-width:74%;font-family:var(--font-sans)}
.sild-msg--in{align-self:flex-start;align-items:flex-start}
.sild-msg--out{align-self:flex-end;align-items:flex-end}
.sild-msg--system{align-self:center;align-items:center;max-width:90%}
.sild-msg__meta{display:flex;align-items:center;gap:7px;margin-bottom:4px;padding:0 4px}
.sild-msg__author{font-size:12px;font-weight:600;color:var(--text-secondary)}
.sild-msg__time{font-size:11px;color:var(--text-tertiary)}
.sild-msg__bubble{font-size:14px;line-height:1.5;padding:9px 13px;border-radius:var(--radius-bubble);
  word-break:break-word;white-space:pre-wrap}
.sild-msg--in .sild-msg__bubble{background:var(--surface-sunken);color:var(--text-primary);border-bottom-left-radius:var(--radius-xs)}
.sild-msg--out .sild-msg__bubble{background:var(--brand);color:#fff;border-bottom-right-radius:var(--radius-xs)}
.sild-msg--internal .sild-msg__bubble{background:var(--warning-subtle);color:var(--slate-800);
  border:1px dashed var(--amber-500);border-radius:var(--radius-md)}
.sild-msg--system .sild-msg__bubble{background:transparent;color:var(--text-tertiary);font-size:12px;padding:4px 8px}
.sild-msg__chan{display:inline-flex;align-items:center;gap:4px;font-size:11px;font-weight:600;color:var(--blue-700);
  background:var(--blue-50);border-radius:var(--radius-full);padding:1px 7px}
.sild-msg__intlabel{display:inline-flex;align-items:center;gap:4px;font-size:11px;font-weight:600;color:var(--amber-600)}
.sild-msg__atts{display:flex;flex-direction:column;gap:6px;margin-top:6px}
.sild-msg__att{display:flex;align-items:center;gap:8px;background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-md);padding:7px 10px;font-size:13px;color:var(--text-primary)}
.sild-msg__att-img{margin-top:6px;border-radius:var(--radius-md);max-width:240px;border:1px solid var(--border-subtle)}
.sild-msg__read{font-size:11px;color:var(--text-tertiary);margin-top:3px;padding:0 4px}
`;
      document.head.appendChild(s);
    }
    const Paperclip = () => /*#__PURE__*/React.createElement("svg", {
      width: "14",
      height: "14",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2",
      strokeLinecap: "round",
      strokeLinejoin: "round"
    }, /*#__PURE__*/React.createElement("path", {
      d: "m21.44 11.05-9.19 9.19a6 6 0 01-8.49-8.49l8.57-8.57A4 4 0 1118 8.84l-8.59 8.57a2 2 0 01-2.83-2.83l8.49-8.48"
    }));
    const Mail = () => /*#__PURE__*/React.createElement("svg", {
      width: "12",
      height: "12",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2",
      strokeLinecap: "round",
      strokeLinejoin: "round"
    }, /*#__PURE__*/React.createElement("rect", {
      x: "2",
      y: "4",
      width: "20",
      height: "16",
      rx: "2"
    }), /*#__PURE__*/React.createElement("path", {
      d: "m22 7-10 5L2 7"
    }));
    const LockGlyph = () => /*#__PURE__*/React.createElement("svg", {
      width: "11",
      height: "11",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2.2",
      strokeLinecap: "round",
      strokeLinejoin: "round"
    }, /*#__PURE__*/React.createElement("rect", {
      x: "3",
      y: "11",
      width: "18",
      height: "11",
      rx: "2"
    }), /*#__PURE__*/React.createElement("path", {
      d: "M7 11V7a5 5 0 0110 0v4"
    }));
    function MessageBubble({
      direction = 'in',
      author,
      time,
      body,
      channel,
      internal = false,
      system = false,
      attachments = [],
      readReceipt,
      className = '',
      ...rest
    }) {
      injectCss();
      const kind = system ? 'system' : direction;
      const cls = ['sild-msg', `sild-msg--${kind}`, internal ? 'sild-msg--internal' : '', className].filter(Boolean).join(' ');
      if (system) {
        return /*#__PURE__*/React.createElement("div", _extends({
          className: cls
        }, rest), /*#__PURE__*/React.createElement("div", {
          className: "sild-msg__bubble"
        }, body));
      }
      return /*#__PURE__*/React.createElement("div", _extends({
        className: cls
      }, rest), (author || time || internal || channel === 'email') && /*#__PURE__*/React.createElement("div", {
        className: "sild-msg__meta"
      }, author && /*#__PURE__*/React.createElement("span", {
        className: "sild-msg__author"
      }, author), internal && /*#__PURE__*/React.createElement("span", {
        className: "sild-msg__intlabel"
      }, /*#__PURE__*/React.createElement(LockGlyph, null), " Internal note"), channel === 'email' && /*#__PURE__*/React.createElement("span", {
        className: "sild-msg__chan"
      }, /*#__PURE__*/React.createElement(Mail, null), " Email"), time && /*#__PURE__*/React.createElement("span", {
        className: "sild-msg__time"
      }, time)), /*#__PURE__*/React.createElement("div", {
        className: "sild-msg__bubble"
      }, body, attachments.filter(a => a.disposition === 'inline' && a.kind === 'image').map((a, i) => /*#__PURE__*/React.createElement("img", {
        key: i,
        className: "sild-msg__att-img",
        src: a.url,
        alt: a.filename || ''
      }))), attachments.filter(a => a.disposition !== 'inline' || a.kind !== 'image').length > 0 && /*#__PURE__*/React.createElement("div", {
        className: "sild-msg__atts"
      }, attachments.filter(a => a.disposition !== 'inline' || a.kind !== 'image').map((a, i) => /*#__PURE__*/React.createElement("span", {
        key: i,
        className: "sild-msg__att"
      }, /*#__PURE__*/React.createElement(Paperclip, null), " ", a.filename || 'attachment'))), readReceipt && /*#__PURE__*/React.createElement("div", {
        className: "sild-msg__read"
      }, readReceipt));
    }
    return MessageBubble;
  }();

  // ---- ConversationRow ----
  window.Sild.ConversationRow = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-convrow{display:flex;gap:11px;align-items:flex-start;padding:11px 14px;cursor:pointer;font-family:var(--font-sans);
  border-left:2px solid transparent;transition:background var(--duration-fast)}
.sild-convrow:hover{background:var(--surface-hover)}
.sild-convrow--active{background:var(--surface-selected);border-left-color:var(--brand)}
.sild-convrow--active:hover{background:var(--surface-selected)}
.sild-convrow__main{flex:1;min-width:0}
.sild-convrow__top{display:flex;align-items:baseline;justify-content:space-between;gap:8px}
.sild-convrow__name{font-size:14px;font-weight:600;color:var(--text-primary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.sild-convrow__name--unread{font-weight:700}
.sild-convrow__time{font-size:11px;color:var(--text-tertiary);flex:none}
.sild-convrow__preview{font-size:13px;color:var(--text-secondary);margin-top:2px;display:-webkit-box;-webkit-line-clamp:1;-webkit-box-orient:vertical;overflow:hidden}
.sild-convrow__preview--unread{color:var(--text-primary)}
.sild-convrow__sub{display:flex;align-items:center;gap:7px;margin-top:6px}
.sild-convrow__ref{font-family:var(--font-mono);font-size:11px;color:var(--text-tertiary);background:var(--surface-sunken);padding:1px 6px;border-radius:var(--radius-xs)}
.sild-convrow__chan{display:inline-flex;color:var(--text-tertiary)}
.sild-convrow__right{display:flex;flex-direction:column;align-items:flex-end;gap:6px;flex:none}
.sild-convrow__count{min-width:18px;height:18px;padding:0 5px;border-radius:var(--radius-full);background:var(--accent);
  color:#fff;font-size:11px;font-weight:700;display:inline-flex;align-items:center;justify-content:center}
`;
      document.head.appendChild(s);
    }
    const Mail = () => /*#__PURE__*/React.createElement("svg", {
      width: "13",
      height: "13",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2",
      strokeLinecap: "round",
      strokeLinejoin: "round"
    }, /*#__PURE__*/React.createElement("rect", {
      x: "2",
      y: "4",
      width: "20",
      height: "16",
      rx: "2"
    }), /*#__PURE__*/React.createElement("path", {
      d: "m22 7-10 5L2 7"
    }));
    function ConversationRow({
      name,
      preview,
      time,
      unread = 0,
      active = false,
      channel = 'app',
      reference,
      presence = null,
      src,
      status,
      onClick,
      className = '',
      ...rest
    }) {
      injectCss();
      const isUnread = unread > 0;
      return /*#__PURE__*/React.createElement("div", _extends({
        className: ['sild-convrow', active ? 'sild-convrow--active' : '', className].filter(Boolean).join(' '),
        onClick: onClick,
        role: "button",
        tabIndex: 0
      }, rest), /*#__PURE__*/React.createElement(Avatar, {
        name: name,
        src: src,
        presence: presence,
        size: "md"
      }), /*#__PURE__*/React.createElement("div", {
        className: "sild-convrow__main"
      }, /*#__PURE__*/React.createElement("div", {
        className: "sild-convrow__top"
      }, /*#__PURE__*/React.createElement("span", {
        className: ['sild-convrow__name', isUnread ? 'sild-convrow__name--unread' : ''].filter(Boolean).join(' ')
      }, name), time && /*#__PURE__*/React.createElement("span", {
        className: "sild-convrow__time"
      }, time)), /*#__PURE__*/React.createElement("div", {
        className: ['sild-convrow__preview', isUnread ? 'sild-convrow__preview--unread' : ''].filter(Boolean).join(' ')
      }, preview), (reference || channel === 'email' || status) && /*#__PURE__*/React.createElement("div", {
        className: "sild-convrow__sub"
      }, channel === 'email' && /*#__PURE__*/React.createElement("span", {
        className: "sild-convrow__chan"
      }, /*#__PURE__*/React.createElement(Mail, null)), reference && /*#__PURE__*/React.createElement("span", {
        className: "sild-convrow__ref"
      }, reference), status)), /*#__PURE__*/React.createElement("div", {
        className: "sild-convrow__right"
      }, isUnread && /*#__PURE__*/React.createElement("span", {
        className: "sild-convrow__count"
      }, unread > 99 ? '99+' : unread)));
    }
    return ConversationRow;
  }();

  // ---- ComposerBar ----
  window.Sild.ComposerBar = function () {
    let _injected = false;
    function injectCss() {
      if (_injected || typeof document === 'undefined') return;
      _injected = true;
      const s = document.createElement('style');
      s.textContent = `
.sild-composer{font-family:var(--font-sans);background:var(--white);border:1px solid var(--border-default);
  border-radius:var(--radius-lg);transition:border-color var(--duration-fast),box-shadow var(--duration-fast)}
.sild-composer:focus-within{border-color:var(--border-focus);box-shadow:var(--ring)}
.sild-composer--internal{background:var(--warning-subtle);border-color:var(--amber-500)}
.sild-composer--internal:focus-within{box-shadow:0 0 0 3px rgba(245,165,36,.28)}
.sild-composer__bar{display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid var(--border-subtle)}
.sild-composer--internal .sild-composer__bar{border-bottom-color:rgba(245,165,36,.35)}
.sild-composer__seg{display:inline-flex;background:var(--surface-sunken);border-radius:var(--radius-md);padding:2px;gap:2px}
.sild-composer__tab{border:0;background:transparent;font-family:inherit;font-size:12px;font-weight:600;
  color:var(--text-secondary);padding:4px 10px;border-radius:var(--radius-sm);cursor:pointer}
.sild-composer__tab--on{background:var(--white);color:var(--text-primary);box-shadow:var(--shadow-xs)}
.sild-composer--internal .sild-composer__tab--on{background:var(--amber-500);color:#fff}
.sild-composer__spacer{flex:1}
.sild-composer__row{display:flex;align-items:flex-end;gap:8px;padding:8px 10px}
.sild-composer__input{flex:1;border:0;outline:none;background:transparent;resize:none;font-family:inherit;
  font-size:14px;line-height:1.5;color:var(--text-primary);max-height:140px;padding:6px 2px}
.sild-composer__input::placeholder{color:var(--text-tertiary)}
.sild-composer__icon{display:inline-flex;align-items:center;justify-content:center;width:34px;height:34px;border-radius:var(--radius-md);
  border:0;background:transparent;color:var(--text-tertiary);cursor:pointer;flex:none;transition:background var(--duration-fast),color var(--duration-fast)}
.sild-composer__icon:hover{background:var(--surface-hover);color:var(--text-primary)}
.sild-composer__send{background:var(--brand);color:#fff}
.sild-composer__send:hover{background:var(--brand-hover);color:#fff}
.sild-composer__send:disabled{opacity:.4;cursor:not-allowed;background:var(--brand)}
.sild-composer--internal .sild-composer__send{background:var(--amber-500)}
.sild-composer--internal .sild-composer__send:hover{background:var(--amber-600)}
`;
      document.head.appendChild(s);
    }
    const Paperclip = () => /*#__PURE__*/React.createElement("svg", {
      width: "18",
      height: "18",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2",
      strokeLinecap: "round",
      strokeLinejoin: "round"
    }, /*#__PURE__*/React.createElement("path", {
      d: "m21.44 11.05-9.19 9.19a6 6 0 01-8.49-8.49l8.57-8.57A4 4 0 1118 8.84l-8.59 8.57a2 2 0 01-2.83-2.83l8.49-8.48"
    }));
    const Send = () => /*#__PURE__*/React.createElement("svg", {
      width: "18",
      height: "18",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2",
      strokeLinecap: "round",
      strokeLinejoin: "round"
    }, /*#__PURE__*/React.createElement("path", {
      d: "M22 2 11 13M22 2l-7 20-4-9-9-4 20-7"
    }));
    function ComposerBar({
      value,
      onChange,
      onSend,
      onAttach,
      placeholder,
      internal = false,
      onToggleInternal,
      showInternalToggle = false,
      disabled = false,
      className = '',
      ...rest
    }) {
      injectCss();
      const ref = React.useRef(null);
      const [internalState, setInternalState] = React.useState(false);
      const isInternal = onToggleInternal ? internal : internalState;
      const setInternal = onToggleInternal || setInternalState;
      React.useEffect(() => {
        const el = ref.current;
        if (el) {
          el.style.height = 'auto';
          el.style.height = Math.min(el.scrollHeight, 140) + 'px';
        }
      }, [value]);
      const ph = placeholder || (isInternal ? 'Add an internal note (only your team sees this)…' : 'Write a reply…');
      const handleKey = e => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          if (value && value.trim() && onSend) onSend();
        }
      };
      return /*#__PURE__*/React.createElement("div", _extends({
        className: ['sild-composer', isInternal ? 'sild-composer--internal' : '', className].filter(Boolean).join(' ')
      }, rest), showInternalToggle && /*#__PURE__*/React.createElement("div", {
        className: "sild-composer__bar"
      }, /*#__PURE__*/React.createElement("div", {
        className: "sild-composer__seg"
      }, /*#__PURE__*/React.createElement("button", {
        type: "button",
        className: ['sild-composer__tab', !isInternal ? 'sild-composer__tab--on' : ''].filter(Boolean).join(' '),
        onClick: () => setInternal(false)
      }, "Reply"), /*#__PURE__*/React.createElement("button", {
        type: "button",
        className: ['sild-composer__tab', isInternal ? 'sild-composer__tab--on' : ''].filter(Boolean).join(' '),
        onClick: () => setInternal(true)
      }, "Internal note")), /*#__PURE__*/React.createElement("span", {
        className: "sild-composer__spacer"
      })), /*#__PURE__*/React.createElement("div", {
        className: "sild-composer__row"
      }, /*#__PURE__*/React.createElement("button", {
        type: "button",
        className: "sild-composer__icon",
        "aria-label": "Attach file",
        onClick: onAttach
      }, /*#__PURE__*/React.createElement(Paperclip, null)), /*#__PURE__*/React.createElement("textarea", {
        ref: ref,
        className: "sild-composer__input",
        rows: 1,
        placeholder: ph,
        value: value,
        disabled: disabled,
        onChange: e => onChange && onChange(e.target.value, e),
        onKeyDown: handleKey
      }), /*#__PURE__*/React.createElement("button", {
        type: "button",
        className: "sild-composer__icon sild-composer__send",
        "aria-label": "Send",
        disabled: disabled || !value || !value.trim(),
        onClick: onSend
      }, /*#__PURE__*/React.createElement(Send, null))));
    }
    return ComposerBar;
  }();
})();
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/sild-bundle.jsx", error: String((e && e.message) || e) }); }

// ui_kits/widget/Widget.jsx
try { (() => {
/* Sild web chat widget — drop-in recreation (spec §9).
   The end-user widget: floating launcher → home → conversation thread.
   Composes window.Sild + window.I. */
const {
  useState,
  useRef,
  useEffect
} = React;
const W = window.Sild,
  Ic = window.I;
const {
  Avatar,
  Button,
  MessageBubble,
  ComposerBar,
  ConversationRow,
  Badge
} = W;
const MARK = '../../assets/sild-mark-tile.svg';
const seedThread = [{
  id: 1,
  dir: 'out',
  author: 'You',
  time: 'Just now',
  body: 'Hi — how do I change my pickup address after booking?'
}, {
  id: 2,
  dir: 'in',
  author: 'Eva · Sild',
  time: 'Just now',
  body: 'Hey! Open your trip, tap the pickup pin, and drag it or search a new address. Changes sync to your driver instantly.'
}];
function Launcher({
  open,
  onClick
}) {
  return /*#__PURE__*/React.createElement("button", {
    onClick: onClick,
    "aria-label": open ? 'Close chat' : 'Open chat',
    style: {
      position: 'absolute',
      right: 24,
      bottom: 24,
      width: 60,
      height: 60,
      borderRadius: '50%',
      border: 0,
      background: 'var(--brand)',
      color: '#fff',
      cursor: 'pointer',
      boxShadow: 'var(--shadow-launcher)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      transition: 'transform var(--duration-base) var(--ease-spring)',
      transform: open ? 'scale(0.92)' : 'scale(1)'
    }
  }, open ? Ic.X({
    width: 26,
    height: 26
  }) : /*#__PURE__*/React.createElement("svg", {
    width: "27",
    height: "27",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"
  })));
}
function Home({
  onStart,
  onOpenThread
}) {
  return /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("div", {
    style: {
      background: 'var(--brand)',
      color: '#fff',
      padding: '22px 22px 26px'
    }
  }, /*#__PURE__*/React.createElement("img", {
    src: MARK,
    width: "34",
    alt: "Sild",
    style: {
      borderRadius: 10,
      boxShadow: '0 0 0 2px rgba(255,255,255,.25)'
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 22,
      fontWeight: 800,
      letterSpacing: '-.02em',
      marginTop: 14
    }
  }, "Hi there."), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 15,
      color: 'rgba(255,255,255,.85)',
      marginTop: 4
    }
  }, "How can we help? We typically reply in a few minutes.")), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      overflowY: 'auto',
      padding: 16,
      display: 'flex',
      flexDirection: 'column',
      gap: 12,
      background: 'var(--surface-page)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      background: 'var(--surface-card)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      boxShadow: 'var(--shadow-sm)',
      padding: 16
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 14,
      fontWeight: 700
    }
  }, "Send us a message"), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 13,
      color: 'var(--text-secondary)',
      marginTop: 3,
      marginBottom: 13
    }
  }, "We'll get back to you here. No queue numbers."), /*#__PURE__*/React.createElement(Button, {
    fullWidth: true,
    onClick: onStart,
    iconRight: /*#__PURE__*/React.createElement("svg", {
      width: "16",
      height: "16",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2",
      strokeLinecap: "round",
      strokeLinejoin: "round"
    }, /*#__PURE__*/React.createElement("path", {
      d: "M5 12h14M12 5l7 7-7 7"
    }))
  }, "New conversation")), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 11,
      fontWeight: 600,
      letterSpacing: '.04em',
      textTransform: 'uppercase',
      color: 'var(--text-tertiary)',
      padding: '4px 4px 0'
    }
  }, "Recent"), /*#__PURE__*/React.createElement("div", {
    style: {
      background: 'var(--surface-card)',
      border: '1px solid var(--border-default)',
      borderRadius: 'var(--radius-lg)',
      boxShadow: 'var(--shadow-sm)',
      overflow: 'hidden'
    }
  }, /*#__PURE__*/React.createElement(ConversationRow, {
    name: "Eva \xB7 Sild",
    preview: "\u2026drag the pickup pin or search a new address.",
    time: "2m",
    presence: "online",
    onClick: onOpenThread
  }))));
}
function Thread({
  msgs,
  onSend,
  onBack
}) {
  const [text, setText] = useState('');
  const scroller = useRef(null);
  useEffect(() => {
    if (scroller.current) scroller.current.scrollTop = scroller.current.scrollHeight;
  }, [msgs.length]);
  return /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("div", {
    style: {
      background: 'var(--brand)',
      color: '#fff',
      padding: '12px 14px',
      display: 'flex',
      alignItems: 'center',
      gap: 10
    }
  }, /*#__PURE__*/React.createElement("button", {
    onClick: onBack,
    "aria-label": "Back",
    style: {
      border: 0,
      background: 'transparent',
      color: '#fff',
      cursor: 'pointer',
      display: 'flex',
      padding: 4,
      borderRadius: 8
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "22",
    height: "22",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round"
  }, /*#__PURE__*/React.createElement("path", {
    d: "M19 12H5M12 19l-7-7 7-7"
  }))), /*#__PURE__*/React.createElement(Avatar, {
    name: "Eva Sild",
    presence: "online",
    size: "md"
  }), /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 15,
      fontWeight: 700,
      letterSpacing: '-.01em'
    }
  }, "Eva \xB7 Sild"), /*#__PURE__*/React.createElement("div", {
    style: {
      fontSize: 12,
      color: 'rgba(255,255,255,.8)'
    }
  }, "Replies in a few minutes"))), /*#__PURE__*/React.createElement("div", {
    ref: scroller,
    style: {
      flex: 1,
      overflowY: 'auto',
      padding: '16px 14px',
      display: 'flex',
      flexDirection: 'column',
      gap: 12,
      background: 'var(--surface-page)'
    }
  }, msgs.map(m => /*#__PURE__*/React.createElement(MessageBubble, {
    key: m.id,
    direction: m.dir,
    author: m.author,
    time: m.time,
    body: m.body,
    readReceipt: m.read
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: '10px 12px 12px',
      background: 'var(--surface-card)',
      borderTop: '1px solid var(--border-default)'
    }
  }, /*#__PURE__*/React.createElement(ComposerBar, {
    value: text,
    onChange: setText,
    placeholder: "Write a message\u2026",
    onSend: () => {
      if (text.trim()) {
        onSend(text);
        setText('');
      }
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      textAlign: 'center',
      fontSize: 11,
      color: 'var(--text-tertiary)',
      marginTop: 8
    }
  }, "Powered by Sild")));
}
function Widget() {
  const [open, setOpen] = useState(true);
  const [view, setView] = useState('home');
  const [msgs, setMsgs] = useState(seedThread);
  const send = body => {
    setMsgs(m => [...m, {
      id: Date.now(),
      dir: 'out',
      author: 'You',
      time: 'Just now',
      body,
      read: 'Delivered'
    }]);
    setTimeout(() => setMsgs(m => [...m, {
      id: Date.now() + 1,
      dir: 'in',
      author: 'Eva · Sild',
      time: 'Just now',
      body: 'Thanks! Let me check that for you — one moment.'
    }]), 900);
  };
  const start = () => {
    setMsgs([]);
    setView('thread');
  };
  return /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      right: 24,
      bottom: 100,
      width: 380,
      height: 600,
      maxHeight: 'calc(100% - 124px)',
      background: 'var(--surface-card)',
      borderRadius: 'var(--radius-xl)',
      boxShadow: 'var(--shadow-widget)',
      overflow: 'hidden',
      display: open ? 'flex' : 'none',
      flexDirection: 'column',
      fontFamily: 'var(--font-sans)',
      transformOrigin: 'bottom right',
      animation: open ? 'sild-widget-in var(--duration-base) var(--ease-out)' : 'none'
    }
  }, view === 'home' ? /*#__PURE__*/React.createElement(Home, {
    onStart: start,
    onOpenThread: () => {
      setMsgs(seedThread);
      setView('thread');
    }
  }) : /*#__PURE__*/React.createElement(Thread, {
    msgs: msgs,
    onSend: send,
    onBack: () => setView('home')
  })), /*#__PURE__*/React.createElement(Launcher, {
    open: open,
    onClick: () => setOpen(o => !o)
  }));
}
ReactDOM.createRoot(document.getElementById('widget-root')).render(/*#__PURE__*/React.createElement(Widget, null));
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/widget/Widget.jsx", error: String((e && e.message) || e) }); }

__ds_ns.ComposerBar = __ds_scope.ComposerBar;

__ds_ns.ConversationRow = __ds_scope.ConversationRow;

__ds_ns.MessageBubble = __ds_scope.MessageBubble;

__ds_ns.StatusPill = __ds_scope.StatusPill;

__ds_ns.Avatar = __ds_scope.Avatar;

__ds_ns.AvatarStack = __ds_scope.AvatarStack;

__ds_ns.Badge = __ds_scope.Badge;

__ds_ns.Button = __ds_scope.Button;

__ds_ns.IconButton = __ds_scope.IconButton;

__ds_ns.Spinner = __ds_scope.Spinner;

__ds_ns.Tag = __ds_scope.Tag;

__ds_ns.Banner = __ds_scope.Banner;

__ds_ns.Card = __ds_scope.Card;

__ds_ns.Dialog = __ds_scope.Dialog;

__ds_ns.Tooltip = __ds_scope.Tooltip;

__ds_ns.Checkbox = __ds_scope.Checkbox;

__ds_ns.Input = __ds_scope.Input;

__ds_ns.Select = __ds_scope.Select;

__ds_ns.Switch = __ds_scope.Switch;

__ds_ns.Textarea = __ds_scope.Textarea;

})();
