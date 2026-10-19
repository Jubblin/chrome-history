#!/usr/bin/env python3
"""Render the 440x280 Web Store small promo tile (RGB, no alpha). Usage: make-promo-tile.py OUT.png
Needs Pillow and DejaVu Sans Bold (override with FONT_BOLD / FONT_REG). No network. The icon shapes mirror icon.svg."""
import os, sys
from PIL import Image, ImageDraw, ImageFont

S = 4  # supersample factor
W, H = 440, 280
BG = (0x16, 0x45, 0x8f); LINE = (0xcf, 0xe0, 0xfa); WHITE = (255, 255, 255); ICONBG = (0x1f, 0x5f, 0xbf)
BOLD = os.environ.get('FONT_BOLD', '/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf')
REG = os.environ.get('FONT_REG', '/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf')

img = Image.new('RGB', (W * S, H * S), BG)
d = ImageDraw.Draw(img)
def rr(x, y, w, h, r, c): d.rounded_rectangle([x * S, y * S, (x + w) * S - 1, (y + h) * S - 1], r * S, fill=c)
def ci(cx, cy, r, c): d.ellipse([(cx - r) * S, (cy - r) * S, (cx + r) * S, (cy + r) * S], fill=c)

# icon (128-unit grid from icon.svg) scaled to 120px at (26, 80)
k = 120 / 128; ox, oy = 26, 80
rr(ox, oy, 120, 120, 26 * k, ICONBG)
rr(ox + 38 * k, oy + 24 * k, 8 * k, 80 * k, 4 * k, LINE)
for y in (36, 64, 92): ci(ox + 42 * k, oy + y * k, 11 * k, WHITE)
for y, w in ((31, 44), (59, 32), (87, 40)): rr(ox + 62 * k, oy + y * k, w * k, 10 * k, 5 * k, WHITE)

def font(p, px): return ImageFont.truetype(p, px * S)
tx = 168
for i, line in enumerate(('Chrome History', 'Audit Timeline')):
    d.text((tx * S, (84 + i * 34) * S), line, font=font(BOLD, 26), fill=WHITE)
d.rounded_rectangle([tx * S, 156 * S, (tx + 56) * S, 160 * S - 1], 2 * S, fill=LINE)
for i, line in enumerate(('Your own browsing history,', 'on your device only.')):
    d.text((tx * S, (172 + i * 22) * S), line, font=font(REG, 15), fill=LINE)

img = img.resize((W, H), Image.LANCZOS)
img.save(sys.argv[1], 'PNG', optimize=True)
