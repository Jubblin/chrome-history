#!/usr/bin/env python3
"""Render icon.svg's shapes to PNGs (stdlib only, 4x4 supersampling). Usage: make-icons.py OUTDIR  (shipped icons live in extension/icons/)"""
import struct, sys, zlib, os

# (kind, params, rgb) mirroring icon.svg on a 128 grid
BG = (0x1f, 0x5f, 0xbf); LINE = (0xcf, 0xe0, 0xfa); W = (255, 255, 255)
def rrect(x, y, w, h, r):
    def f(px, py):
        if not (x <= px < x + w and y <= py < y + h): return False
        cx = min(max(px, x + r), x + w - r); cy = min(max(py, y + r), y + h - r)
        return (px - cx) ** 2 + (py - cy) ** 2 <= r * r
    return f
def circ(cx, cy, r): return lambda px, py: (px - cx) ** 2 + (py - cy) ** 2 <= r * r
SHAPES = [(rrect(0, 0, 128, 128, 26), BG), (rrect(38, 24, 8, 80, 4), LINE)]
SHAPES += [(circ(42, y, 11), W) for y in (36, 64, 92)]
SHAPES += [(rrect(62, 31, 44, 10, 5), W), (rrect(62, 59, 32, 10, 5), W), (rrect(62, 87, 40, 10, 5), W)]

def render(n, ss=4):
    k = 128 / n; rows = []
    for j in range(n):
        row = bytearray()
        for i in range(n):
            acc = [0, 0, 0, 0]
            for sj in range(ss):
                for si in range(ss):
                    px = (i + (si + .5) / ss) * k; py = (j + (sj + .5) / ss) * k
                    col = None
                    for f, c in SHAPES:
                        if f(px, py): col = c
                    if col: acc[0] += col[0]; acc[1] += col[1]; acc[2] += col[2]; acc[3] += 255
            t = ss * ss; a = acc[3] // t
            row += bytes([acc[0] // t, acc[1] // t, acc[2] // t, a]) if a == 0 else bytes([acc[0] * 255 // acc[3], acc[1] * 255 // acc[3], acc[2] * 255 // acc[3], a])
        rows.append(b'\x00' + bytes(row))
    def chunk(t, d): return struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d))
    return b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', n, n, 8, 6, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(b''.join(rows), 9)) + chunk(b'IEND', b'')

out = sys.argv[1]; os.makedirs(out, exist_ok=True)
for n in (16, 32, 48, 128):
    open(os.path.join(out, f'icon-{n}.png'), 'wb').write(render(n))
