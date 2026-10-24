"""Sild identity master: one geometry, every asset derived from it. Units = font units, y down, baseline 0."""
import json, math, pathlib
OUT = pathlib.Path(__file__).parent
G = json.load(open(OUT / "glyphs.json"))["g"]

NAVY, BLUE, CORAL, NIGHT, WHITE = "#1B2E70", "#2563FD", "#FF7A45", "#0B1640", "#FFFFFF"
T = -55                                   # tracking
STEM = 350                                # i / l / d stem width
X = {}; x = 0
for ch, gl in (("s", "s"), ("i", "ı"), ("l", "l"), ("d", "d")):
    X[ch] = x; x += G[gl]["adv"] + T
WIDTH = x - T

# The hop: sender is the i-dot, receiver floats above the d stem.
S = (X["i"] + 308.5, -1310.0); RS = 172
R = (X["d"] + 1009.5, -1655.0); RR = 95
SPAN = R[0] - S[0]
CREST = -2210
P1 = (S[0] + 0.03 * SPAN, -2110)
P2 = (R[0] - 0.30 * SPAN, CREST - 170)
ARC_W = 92
SLIT_T, SLIT_ANGLE, SLIT_GAP = 0.55, -62, 46

def bez(t, a=S, b=P1, c=P2, d=R):
    return tuple((1-t)**3*p + 3*t*(1-t)**2*q + 3*t*t*(1-t)*r + t**3*s for p, q, r, s in zip(a, b, c, d))
ARC = f"M{S[0]:.1f} {S[1]:.1f} C{P1[0]:.1f} {P1[1]:.1f} {P2[0]:.1f} {P2[1]:.1f} {R[0]:.1f} {R[1]:.1f}"
SLIT = bez(SLIT_T)

def letters(fill, extra=""):
    return "".join(f'<path transform="translate({X[c]} 0) scale(1 -1)" d="{G[g]["d"]}" fill="{fill}" {extra}/>'
                   for c, g in (("s", "s"), ("i", "ı"), ("l", "l"), ("d", "d")))

def slit_mask(mid):
    a = math.radians(SLIT_ANGLE); L = 600
    dx, dy = math.cos(a) * L / 2, math.sin(a) * L / 2
    return (f'<mask id="{mid}" maskUnits="userSpaceOnUse" x="-2000" y="-4000" width="10000" height="6000">'
            f'<rect x="-2000" y="-4000" width="10000" height="6000" fill="#fff"/>'
            f'<line x1="{SLIT[0]-dx:.1f}" y1="{SLIT[1]-dy:.1f}" x2="{SLIT[0]+dx:.1f}" y2="{SLIT[1]+dy:.1f}" stroke="#000" stroke-width="{SLIT_GAP}"/></mask>')

def blur(fid, sd):
    return f'<filter id="{fid}" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="{sd}"/></filter>'

def slit_line(color, w, scale):
    a = math.radians(SLIT_ANGLE); L = w * scale
    dx, dy = math.cos(a) * L / 2, math.sin(a) * L / 2
    return (f'<line x1="{SLIT[0]-dx:.1f}" y1="{SLIT[1]-dy:.1f}" x2="{SLIT[0]+dx:.1f}" y2="{SLIT[1]+dy:.1f}" '
            f'stroke="{color}" stroke-width="{SLIT_GAP*.42:.0f}" stroke-linecap="round"/>')

def hop(mode, p="h", w=ARC_W, rr=RR, slit=True):
    """The sender, arc and receiver. mode: light | dark | mono | small."""
    defs, body = [], []
    m = f' mask="url(#{p}-slit)"' if slit else ""
    if slit: defs.append(slit_mask(f"{p}-slit"))
    if mode == "light":
        defs += [f'<linearGradient id="{p}-arc" gradientUnits="userSpaceOnUse" x1="{S[0]}" y1="0" x2="{R[0]}" y2="0">'
                 f'<stop offset="0" stop-color="{BLUE}" stop-opacity=".28"/><stop offset=".45" stop-color="{BLUE}" stop-opacity=".85"/>'
                 f'<stop offset="1" stop-color="{BLUE}"/></linearGradient>', blur(f"{p}-b1", 70), blur(f"{p}-b2", 55)]
        body += [f'<path d="{ARC}" fill="none" stroke="{BLUE}" stroke-opacity=".22" stroke-width="{w*2.6}" stroke-linecap="round" filter="url(#{p}-b1)"/>',
                 f'<circle cx="{SLIT[0]:.1f}" cy="{SLIT[1]:.1f}" r="175" fill="{CORAL}" opacity=".85" filter="url(#{p}-b2)"/>',
                 f'<circle cx="{R[0]}" cy="{R[1]}" r="{rr*1.9:.0f}" fill="{CORAL}" opacity=".35" filter="url(#{p}-b2)"/>',
                 f'<path d="{ARC}" fill="none" stroke="url(#{p}-arc)" stroke-width="{w}" stroke-linecap="round"{m}/>',
                 (slit_line(CORAL, w, 2.3) if slit else ""),
                 f'<circle cx="{S[0]}" cy="{S[1]}" r="{RS}" fill="{BLUE}"/>', f'<circle cx="{R[0]}" cy="{R[1]}" r="{rr}" fill="{CORAL}"/>']
    elif mode == "dark":
        defs += [f'<linearGradient id="{p}-arc" gradientUnits="userSpaceOnUse" x1="{S[0]}" y1="0" x2="{R[0]}" y2="0">'
                 f'<stop offset="0" stop-color="#4F86FF"/><stop offset="1" stop-color="#BFD3FF"/></linearGradient>',
                 blur(f"{p}-b1", 60), blur(f"{p}-b2", 50)]
        body += [f'<path d="{ARC}" fill="none" stroke="{BLUE}" stroke-width="{w*3}" stroke-linecap="round" opacity=".55" filter="url(#{p}-b1)"/>',
                 f'<circle cx="{SLIT[0]:.1f}" cy="{SLIT[1]:.1f}" r="170" fill="{CORAL}" opacity=".8" filter="url(#{p}-b2)"/>',
                 f'<circle cx="{R[0]}" cy="{R[1]}" r="{rr*2.2:.0f}" fill="{CORAL}" opacity=".6" filter="url(#{p}-b2)"/>',
                 f'<path d="{ARC}" fill="none" stroke="url(#{p}-arc)" stroke-width="{w}" stroke-linecap="round"{m}/>',
                 f'<path d="{ARC}" fill="none" stroke="#fff" stroke-opacity=".7" stroke-width="{w*.22:.0f}" stroke-linecap="round"{m}/>',
                 (slit_line("#FFE3D6", w, 2.6) if slit else ""),
                 f'<circle cx="{S[0]}" cy="{S[1]}" r="{RS}" fill="#2D6BFF" stroke="#8DB0FF" stroke-width="16"/>',
                 f'<circle cx="{R[0]}" cy="{R[1]}" r="{rr}" fill="{CORAL}" stroke="#FFC3A8" stroke-width="12"/>']
    elif mode == "small":
        body += [f'<path d="{ARC}" fill="none" stroke="{BLUE}" stroke-width="{w}" stroke-linecap="round"{m}/>',
                 f'<circle cx="{S[0]}" cy="{S[1]}" r="{RS}" fill="{BLUE}"/>', f'<circle cx="{R[0]}" cy="{R[1]}" r="{rr}" fill="{CORAL}"/>']
    else:  # mono
        body += [f'<path d="{ARC}" fill="none" stroke="currentColor" stroke-width="{w}" stroke-linecap="round"{m}/>',
                 f'<circle cx="{S[0]}" cy="{S[1]}" r="{RS}" fill="currentColor"/>', f'<circle cx="{R[0]}" cy="{R[1]}" r="{rr}" fill="currentColor"/>']
    return "".join(defs), "".join(body)

PAD = 90
WM_VB = (-PAD, CREST - ARC_W - 200, WIDTH + 2 * PAD, -(CREST - ARC_W - 200) + 60 + PAD)

def svg(vb, inner, defs="", extra=""):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{" ".join(f"{v:.0f}" for v in vb)}" {extra}>'
            f'<defs>{defs}</defs>{inner}</svg>')

def wordmark(mode, p="w"):
    if mode == "small":
        d, b = hop("small", p, w=ARC_W * 1.9, rr=RR * 1.45, slit=False); fill = NAVY
    else:
        d, b = hop(mode, p)
        fill = {"light": NAVY, "dark": WHITE, "mono": "currentColor"}[mode]
    extra = ""
    if mode == "dark":
        d += (f'<linearGradient id="{p}-ink" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#3B7BFF"/>'
              f'<stop offset="1" stop-color="#1E50E8"/></linearGradient>' + blur(f"{p}-lg", 45))
        lg = f'<g filter="url(#{p}-lg)" opacity=".75">{letters(BLUE)}</g>'
        return svg(WM_VB, lg + letters(f"url(#{p}-ink)") + b, d)
    return svg(WM_VB, letters(fill) + b, d)

# Symbol: the hop alone, square, sender low-left / receiver high-right as in the wordmark.
def sym_box(pad):
    x0, x1 = S[0] - RS - pad, R[0] + RR * 1.5 + pad
    y0, y1 = CREST - ARC_W - pad, S[1] + RS + pad
    side = max(x1 - x0, y1 - y0); cx, cy = (x0 + x1) / 2, (y0 + y1) / 2
    return (cx - side / 2, cy - side / 2, side, side)

def symbol(mode, p="s", pad=160):
    if mode == "small":
        d, b = hop("small", p, w=ARC_W * 2.1, rr=RR * 1.6, slit=False)
    else:
        d, b = hop(mode, p)
    return svg(sym_box(pad), b, d)

def tile(mode, p="t"):
    vb = sym_box(300); x, y, s, _ = vb
    bg = (f'<rect x="{x}" y="{y}" width="{s}" height="{s}" rx="{s*.2237:.0f}" fill="{NIGHT if mode == "dark" else WHITE}"'
          + ('' if mode == "dark" else ' stroke="#E3E8F2" stroke-width="18"') + "/>")
    d, b = hop(mode, p, w=ARC_W * 1.5, rr=RR * 1.3)
    return svg(vb, bg + b, d)

def favicon(p="f"):
    # Pixel-led: thick white arc, white sender, coral receiver on a navy tile; no slit, no glow.
    vb = sym_box(330); x, y, s, _ = vb
    bg = f'<rect x="{x}" y="{y}" width="{s}" height="{s}" rx="{s*.22:.0f}" fill="{BLUE}"/>'
    w = ARC_W * 2.6
    body = (f'<path d="{ARC}" fill="none" stroke="#fff" stroke-width="{w}" stroke-linecap="round"/>'
            f'<circle cx="{S[0]}" cy="{S[1]}" r="{RS*1.25}" fill="#fff"/><circle cx="{R[0]}" cy="{R[1]}" r="{RR*2.0}" fill="{CORAL}"/>')
    return svg(vb, bg + body)

def connecting():
    # The hop as a loader: a pulse travels sender -> slit -> receiver, receiver lights; static under reduced motion.
    d, b = hop("light", "c")
    x, y, s, _ = sym_box(160)
    anim = (f'<circle r="70" fill="{CORAL}"><animateMotion dur="1.6s" repeatCount="indefinite" path="{ARC}" '
            f'keyPoints="0;1;1" keyTimes="0;.75;1" calcMode="spline" keySplines=".45 0 .25 1;0 0 1 1"/>'
            f'<animate attributeName="opacity" dur="1.6s" repeatCount="indefinite" values="0;1;1;0" keyTimes="0;.1;.72;.8"/></circle>'
            f'<circle cx="{R[0]}" cy="{R[1]}" r="{RR}" fill="{CORAL}"><animate attributeName="r" dur="1.6s" repeatCount="indefinite" '
            f'values="{RR};{RR};{RR*1.45:.0f};{RR}" keyTimes="0;.72;.8;1"/></circle>')
    style = '<style>@media (prefers-reduced-motion: reduce){animate,animateMotion{display:none}}</style>'
    return svg((x, y, s, s), style + b + anim, d)

files = {
    "sild-wordmark-light.svg": wordmark("light", "wl"),
    "sild-wordmark-dark.svg": wordmark("dark", "wd"),
    "sild-wordmark-mono.svg": wordmark("mono", "wm"),
    "sild-wordmark-small.svg": wordmark("small", "wsm"),
    "sild-symbol-light.svg": symbol("light", "sl"),
    "sild-symbol-dark.svg": symbol("dark", "sd"),
    "sild-symbol-mono.svg": symbol("mono", "sm"),
    "sild-symbol-small.svg": symbol("small", "ss"),
    "sild-app-icon-dark.svg": tile("dark", "td"),
    "sild-app-icon-light.svg": tile("light", "tl"),
    "favicon.svg": favicon(),
    "sild-connecting.svg": connecting(),
}
for name, s in files.items():
    (OUT / name).write_text(s)
print("geometry", dict(S=S, R=R, crest=CREST, span=SPAN, slit=SLIT))
