#!/usr/bin/env python3
"""Render the Web Store promo images from one design: promo-tile-440x280.png (small) and promo-marquee-1400x560.png (marquee), RGB, no alpha.
Usage: make-promo-tile.py OUTDIR. Needs Pillow and DejaVu Sans (override with FONT_BOLD / FONT_REG). No network. Icon shapes mirror icon.svg."""
import os, sys
from PIL import Image, ImageDraw, ImageFont

S = 4  # supersample factor
BG = (0x16, 0x45, 0x8f); LINE = (0xcf, 0xe0, 0xfa); WHITE = (255, 255, 255); ICONBG = (0x1f, 0x5f, 0xbf)
BOLD = os.environ.get('FONT_BOLD', '/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf')
REG = os.environ.get('FONT_REG', '/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf')

def render(W, H, u, path):
    """u = scale relative to the 440x280 layout; icon and text positions are laid out in 440x280 units, then centred."""
    img = Image.new('RGB', (W * S, H * S), BG)
    d = ImageDraw.Draw(img)
    def rr(x, y, w, h, r, c): d.rounded_rectangle([x * S, y * S, (x + w) * S - 1, (y + h) * S - 1], r * S, fill=c)
    def ci(cx, cy, r, c): d.ellipse([(cx - r) * S, (cy - r) * S, (cx + r) * S, (cy + r) * S], fill=c)
    def font(p, px): return ImageFont.truetype(p, round(px * u * S))
    # content block is 386 units wide (icon 120 + gap 22 + text 244) and 120 tall in 440x280 units
    ox = (W - 386 * u) / 2; oy = (H - 120 * u) / 2
    sz = 120 * u; k = sz / 128
    rr(ox, oy, sz, sz, 26 * k, ICONBG)
    rr(ox + 38 * k, oy + 24 * k, 8 * k, 80 * k, 4 * k, LINE)
    for y in (36, 64, 92): ci(ox + 42 * k, oy + y * k, 11 * k, WHITE)
    for y, w in ((31, 44), (59, 32), (87, 40)): rr(ox + 62 * k, oy + y * k, w * k, 10 * k, 5 * k, WHITE)
    tx = ox + 142 * u
    for i, line in enumerate(('Browsing History', 'Timeline')):
        d.text((tx * S, (oy + (4 + i * 34) * u) * S), line, font=font(BOLD, 24), fill=WHITE)
    rr(tx, oy + 76 * u, 56 * u, 4 * u, 2 * u, LINE)
    for i, line in enumerate(('Your own browsing history,', 'on your device only.')):
        d.text((tx * S, (oy + (92 + i * 22) * u) * S), line, font=font(REG, 15), fill=LINE)
    img.resize((W, H), Image.LANCZOS).save(path, 'PNG', optimize=True)

out = sys.argv[1]; os.makedirs(out, exist_ok=True)
render(440, 280, 1, os.path.join(out, 'promo-tile-440x280.png'))
render(1400, 560, 2.6, os.path.join(out, 'promo-marquee-1400x560.png'))
