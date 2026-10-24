# Sild brand

The mark is the hop: the i-dot (sender) sends over the l to a coral receiver above the d, crossing one slit — Sild
connecting peers, apps, APIs and platforms. Lettering is Schibsted Grotesk 800 (OFL).

Colours: navy `#1B2E70` · signal blue `#2563FD` · coral `#FF7A45` · night `#0B1640`.

## Which file where

| Use | File |
|---|---|
| App / SDK icon, GitHub avatar, inbox sidebar | `renders/sild-app-icon-dark.png` (inbox: `inbox/public/assets/sild-mark-tile.png`) |
| Hero, marketing, docs splash — on white | `renders/sild-wordmark-light.png` |
| Hero, marketing — on dark | `renders/sild-wordmark-dark.png` |
| Favicon | `vector/favicon.svg` (inbox: `inbox/src/app/icon.svg`) |
| Product headers on white, ≥32px tall | `vector/sild-wordmark-light.svg` |
| Product headers below 32px tall | `vector/sild-wordmark-small.svg` |
| Dark-mode headers, print, partner logos | `vector/sild-wordmark-mono.svg` (one colour) |
| "Connecting…" state | `vector/sild-connecting.svg`; motion reference `renders/sild-connecting-storyboard.png` |

`renders/` are the generated masters — the look of the brand, fixed-size rasters. `vector/` is the scalable
redraw for small sizes, one-colour use and animation; it is flatter than the renders, so prefer a render wherever a
bitmap at that size is acceptable.

## Regenerating the vectors

All vector files come from one geometry in `vector/build.py` (`python3 build.py`). `glyphs.json` holds the
Schibsted Grotesk 800 outlines for s, ı, l, d, extracted with fontTools from the variable font at `wght=800`.
