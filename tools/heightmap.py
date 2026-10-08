#!/usr/bin/env python3
"""Derive a heightmap from a painted wall texture: assets/walls/NAME.png -> assets/walls/NAME_height.png.

Dark crevices (mortar, cracks, plank gaps) are the floor; every other pixel rises with its distance to
the nearest crevice, up to a plateau after BEVEL px, plus a little of the texture's own fine detail.
The texture is seamless, so every filter wraps around.

usage: tools/heightmap.py NAME... (default: every wall in walls.txt plus stone)"""
import sys
from pathlib import Path

import numpy as np
from PIL import Image

WALLS = Path(__file__).resolve().parent.parent / "assets" / "walls"
BEVEL = 6      # px from a crevice to the plateau
CREVICE = 0.6  # pixels darker than local mean - CREVICE*local std are crevice
MIN_SEAM = 200 # crevice regions smaller than this (px) are surface detail, not mortar
DETAIL = 0.15  # share of fine luminance detail kept on top of the blocks


def blur(a, r):
    """Wrapping box blur, radius r, applied twice (close to a gaussian)."""
    for _ in range(2):
        for ax in (0, 1):
            a = sum(np.roll(a, s, ax) for s in range(-r, r + 1)) / (2 * r + 1)
    return a


def distance(seam):
    """Chamfer distance (1, 1.4) to the nearest seam pixel, wrapping."""
    d = np.where(seam, 0.0, 1e9)
    for _ in range(BEVEL + 2):  # beyond BEVEL the value is clamped anyway
        n = d
        for dy, dx, w in ((0, 1, 1), (0, -1, 1), (1, 0, 1), (-1, 0, 1),
                          (1, 1, 1.4), (1, -1, 1.4), (-1, 1, 1.4), (-1, -1, 1.4)):
            n = np.minimum(n, np.roll(d, (dy, dx), (0, 1)) + w)
        d = n
    return d


def large(seam):
    """Keep connected seam regions of at least MIN_SEAM px (label propagation, wrapping)."""
    lab = np.where(seam, np.arange(seam.size).reshape(seam.shape), -1)
    while True:
        n = lab
        for sh in ((0, 1), (0, -1), (1, 0), (-1, 0)):
            n = np.maximum(n, np.roll(lab, sh, (0, 1)))
        n = np.where(seam, n, -1)
        if (n == lab).all():
            break
        lab = n
    ids, counts = np.unique(lab[seam], return_counts=True)
    return np.isin(lab, ids[counts >= MIN_SEAM]) & seam


def height(img):
    lum = np.asarray(img.convert("L"), dtype=float) / 255
    mean = blur(lum, 6)
    std = np.sqrt(np.maximum(blur((lum - mean) ** 2, 6), 1e-6))
    seam = lum < mean - CREVICE * std
    seam &= blur(seam.astype(float), 1) > 0.3  # drop speckle, keep lines
    seam = large(seam)
    block = np.clip((distance(seam) - 1) / BEVEL, 0, 1)
    detail = lum - blur(lum, 2)
    return np.clip(block * (1 - DETAIL) + DETAIL * (0.5 + 2 * detail), 0, 1)


def main(names):
    if not names:
        names = ["stone"] + [l.split()[1] for l in (WALLS / "walls.txt").read_text().splitlines()
                             if l.strip() and not l.startswith("#")]
    for n in names:
        h = height(Image.open(WALLS / f"{n}.png"))
        Image.fromarray((h * 255).astype(np.uint8)).save(WALLS / f"{n}_height.png")
        print(f"{n}_height.png")


if __name__ == "__main__":
    main(sys.argv[1:])
