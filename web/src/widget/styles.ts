// All widget CSS, injected into the shadow root so nothing leaks in or out
// (§9 "shadow DOM for style isolation"). Tokens are defined on :host. Ported
// from the Sild design system + the widget surface of the source design.
export const css = `
:host {
  --brand: #2563FD;
  --brand-hover: #1A4FE0;
  --white: #FFFFFF;
  --slate-50: #F6F8FA;
  --slate-100: #EDF0F4;
  --slate-200: #DDE3EA;
  --slate-400: #98A2B3;
  --slate-500: #6B7585;
  --slate-600: #4B5563;
  --slate-900: #14181F;
  --surface-page: #F6F8FA;
  --surface-card: #FFFFFF;
  --surface-sunken: #EDF0F4;
  --text-primary: #14181F;
  --text-secondary: #4B5563;
  --text-tertiary: #6B7585;
  --border-default: #DDE3EA;
  --border-subtle: #EDF0F4;
  --radius-md: 8px;
  --radius-lg: 12px;
  --radius-xl: 16px;
  --radius-bubble: 18px;
  --radius-xs: 4px;
  --font-sans: 'Schibsted Grotesk', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  --shadow-sm: 0 1px 3px rgba(20,24,31,.08), 0 1px 2px rgba(20,24,31,.05);
  --shadow-widget: 0 16px 48px rgba(20,24,31,.22), 0 4px 12px rgba(20,24,31,.10);
  --shadow-launcher: 0 8px 24px rgba(43,78,230,.36), 0 2px 6px rgba(20,24,31,.16);

  position: fixed;
  right: 24px;
  bottom: 24px;
  z-index: 2147483000;
  font-family: var(--font-sans);
}
*, *::before, *::after { box-sizing: border-box; }

.launcher {
  position: fixed; right: 24px; bottom: 24px;
  width: 60px; height: 60px; border-radius: 50%; border: 0;
  background: var(--brand); color: #fff; cursor: pointer;
  box-shadow: var(--shadow-launcher);
  display: flex; align-items: center; justify-content: center;
  transition: transform .2s cubic-bezier(.34,1.56,.64,1);
}
.launcher:hover { transform: scale(1.05); }
.launcher.open { transform: scale(.92); }

.panel {
  position: fixed; right: 24px; bottom: 100px;
  width: 380px; height: 600px; max-height: calc(100vh - 124px); max-width: calc(100vw - 48px);
  background: var(--surface-card); border-radius: var(--radius-xl);
  box-shadow: var(--shadow-widget); overflow: hidden;
  display: flex; flex-direction: column;
  transform-origin: bottom right;
  animation: sild-in .22s cubic-bezier(.16,1,.3,1);
}
@keyframes sild-in { from { opacity: 0; transform: translateY(10px) scale(.97); } to { opacity: 1; transform: none; } }

.brandhead { background: var(--brand); color: #fff; padding: 22px 22px 26px; flex: none; }
.brandhead .tile { width: 34px; height: 34px; border-radius: 10px; box-shadow: 0 0 0 2px rgba(255,255,255,.25); display: block; }
.brandhead h1 { margin: 14px 0 0; font-size: 22px; font-weight: 800; letter-spacing: -.02em; }
.brandhead p { margin: 4px 0 0; font-size: 15px; color: rgba(255,255,255,.85); }

.threadhead { background: var(--brand); color: #fff; padding: 12px 14px; display: flex; align-items: center; gap: 10px; flex: none; }
.threadhead .name { font-size: 15px; font-weight: 700; letter-spacing: -.01em; }
.threadhead .sub { font-size: 12px; color: rgba(255,255,255,.8); }
.iconbtn { border: 0; background: transparent; color: #fff; cursor: pointer; display: flex; padding: 4px; border-radius: 8px; }
.iconbtn:hover { background: rgba(255,255,255,.15); }
.av { width: 34px; height: 34px; border-radius: 50%; background: rgba(255,255,255,.18); display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 13px; flex: none; }

.body { flex: 1; min-height: 0; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 12px; background: var(--surface-page); }

.card { background: var(--surface-card); border: 1px solid var(--border-default); border-radius: var(--radius-lg); box-shadow: var(--shadow-sm); padding: 16px; }
.card h2 { margin: 0; font-size: 14px; font-weight: 700; color: var(--text-primary); }
.card p { margin: 3px 0 13px; font-size: 13px; color: var(--text-secondary); }

.btn { width: 100%; height: 40px; border: 0; border-radius: var(--radius-md); background: var(--brand); color: #fff; font-family: inherit; font-size: 14px; font-weight: 600; cursor: pointer; display: inline-flex; align-items: center; justify-content: center; gap: 6px; }
.btn:hover { background: var(--brand-hover); }

.eyebrow { font-size: 11px; font-weight: 600; letter-spacing: .04em; text-transform: uppercase; color: var(--text-tertiary); padding: 4px 4px 0; }

.row { display: flex; gap: 11px; align-items: flex-start; padding: 12px 14px; cursor: pointer; background: var(--surface-card); border: 1px solid var(--border-default); border-radius: var(--radius-lg); box-shadow: var(--shadow-sm); }
.row:hover { background: var(--slate-50); }
.row .name { font-size: 14px; font-weight: 600; color: var(--text-primary); }
.row .prev { font-size: 13px; color: var(--text-secondary); margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row .time { font-size: 11px; color: var(--text-tertiary); flex: none; }

.msg { display: flex; flex-direction: column; max-width: 80%; }
.msg.in { align-self: flex-start; align-items: flex-start; }
.msg.out { align-self: flex-end; align-items: flex-end; }
.msg.system { align-self: center; align-items: center; max-width: 90%; }
.msg .meta { display: flex; gap: 7px; align-items: center; margin-bottom: 4px; padding: 0 4px; }
.msg .author { font-size: 12px; font-weight: 600; color: var(--text-secondary); }
.msg .mtime { font-size: 11px; color: var(--text-tertiary); }
.bubble { font-size: 14px; line-height: 1.5; padding: 9px 13px; border-radius: var(--radius-bubble); white-space: pre-wrap; word-break: break-word; }
.msg.in .bubble { background: var(--surface-sunken); color: var(--text-primary); border-bottom-left-radius: var(--radius-xs); }
.msg.out .bubble { background: var(--brand); color: #fff; border-bottom-right-radius: var(--radius-xs); }
.msg.system .bubble { background: transparent; color: var(--text-tertiary); font-size: 12px; padding: 4px 8px; }

.composer { padding: 10px 12px 12px; background: var(--surface-card); border-top: 1px solid var(--border-default); flex: none; }
.inputwrap { display: flex; align-items: flex-end; gap: 8px; background: var(--white); border: 1px solid var(--border-default); border-radius: var(--radius-lg); padding: 6px 8px; transition: border-color .14s, box-shadow .14s; }
.inputwrap:focus-within { border-color: var(--brand); box-shadow: 0 0 0 3px rgba(37,99,253,.32); }
.inputwrap textarea { flex: 1; border: 0; outline: none; resize: none; background: transparent; font-family: inherit; font-size: 14px; line-height: 1.5; color: var(--text-primary); max-height: 120px; padding: 6px 2px; }
.send { width: 34px; height: 34px; flex: none; border: 0; border-radius: var(--radius-md); background: var(--brand); color: #fff; cursor: pointer; display: flex; align-items: center; justify-content: center; }
.send:hover { background: var(--brand-hover); }
.send:disabled { opacity: .4; cursor: not-allowed; }
.powered { text-align: center; font-size: 11px; color: var(--text-tertiary); margin-top: 8px; }

.note { font-size: 12px; color: var(--text-tertiary); text-align: center; padding: 8px 16px; }
.banner { margin: 0 0 4px; background: var(--surface-sunken); color: var(--text-secondary); font-size: 12px; border-radius: var(--radius-md); padding: 8px 10px; text-align: center; }
`;
