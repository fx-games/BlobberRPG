#!/usr/bin/env python3
"""Generate floor, wall or ceiling textures with the OpenAI image API.
  python3 tools/new-texture.py
Uses $OPENAI_API_KEY, else asks once (kept in ~/.config/blobber/openai_key). Writes
assets/walls/<kind>_<name>.png (256x256, made seamless). A wall also gets a map character in
assets/walls/walls.txt, so it shows up in the editor's material palette; floors and ceilings show up
in the editor's floor/ceiling row (a map picks one with a "floor NAME" / "ceiling NAME" line).
OPENAI_IMAGE_MODEL overrides the model."""
import base64, io, json, os, re, string, sys, urllib.error, urllib.request
from pathlib import Path
import numpy as np
from PIL import Image

MODEL = os.environ.get("OPENAI_IMAGE_MODEL", "gpt-image-2")
KEYFILE = Path.home() / ".config/blobber/openai_key"
ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "assets/walls"
TABLE = OUT / "walls.txt"
RAW = Path.home() / "Pictures/blobber-textures"  # untouched originals and tiled previews
SIZE = 256
RESERVED = set(".#DLBMWI")  # floor, stone, door, locked door, generated materials

PROMPT = ("Give me a slick square, repeatable dungeon {kind} texture: {desc}. Make it bearable to look "
          "at even with infinite repetition. {view} Seamless tiling on all four edges, even flat lighting, "
          "no vignette, no single light source, no objects, nothing that stands out.")
VIEW = {"floor": "Seen from directly above.",
        "ceiling": "Seen from directly below.",
        "wall": "A flat wall seen straight on, no perspective, filling the whole square."}


def api_key():
    if os.environ.get("OPENAI_API_KEY"):
        return os.environ["OPENAI_API_KEY"]
    if KEYFILE.exists():
        return KEYFILE.read_text().strip()
    key = input("OpenAI API key: ").strip()
    KEYFILE.parent.mkdir(parents=True, exist_ok=True)
    KEYFILE.write_text(key)
    KEYFILE.chmod(0o600)
    return key


def generate(key, prompt):
    body = {"model": MODEL, "prompt": prompt, "size": "1024x1024", "n": 1}
    req = urllib.request.Request("https://api.openai.com/v1/images/generations", json.dumps(body).encode(),
                                 {"Content-Type": "application/json", "Authorization": f"Bearer {key}"})
    try:
        with urllib.request.urlopen(req, timeout=300) as r:
            return base64.b64decode(json.load(r)["data"][0]["b64_json"])
    except urllib.error.HTTPError as e:
        sys.exit(f"OpenAI error {e.code}: {e.read().decode()}")


def seamless(img):
    """Cross-fades the image with a copy shifted by half its size: the copy's edges wrap cleanly,
    and it only shows near the edges, so the original's own seam stays hidden."""
    a = np.asarray(img.convert("RGB"), dtype=np.float32)
    h, w, _ = a.shape
    shifted = np.roll(a, (h // 2, w // 2), axis=(0, 1))
    ramp = lambda n: 1 - np.abs(np.linspace(-1, 1, n))  # 0 at the edges, 1 in the middle
    wgt = np.clip(np.minimum.outer(ramp(h), ramp(w)) * 3, 0, 1)[..., None]
    return Image.fromarray((a * wgt + shifted * (1 - wgt)).astype(np.uint8))


def free_char():
    used = RESERVED | {l.split()[0] for l in TABLE.read_text().splitlines() if l.strip() and not l.startswith("#")}
    return next(c for c in string.ascii_uppercase + string.digits if c not in used)


def create(key):
    kind = {"f": "floor", "w": "wall", "c": "ceiling"}.get(input("Texture: [f]loor, [w]all or [c]eiling? ").strip().lower()[:1])
    if not kind:
        return
    desc = input(f"Describe the {kind} (e.g. 'dirty and dusty, hard to tell stone covered with dirt from dirt covered with stone'): ").strip()
    default = re.sub(r"[^a-z0-9]+", "_", desc.lower()).strip("_")[:24].strip("_")
    name = input(f"Name [{default}]: ").strip() or default
    dest = OUT / f"{kind}_{name}.png"
    if dest.exists() and input(f"{dest} exists, overwrite? [y/N] ").lower() != "y":
        return
    raw = RAW / f"{kind}_{name}"
    raw.mkdir(parents=True, exist_ok=True)
    while True:
        print(f"Generating {kind} texture…")
        png = generate(key, PROMPT.format(kind=kind, desc=desc, view=VIEW[kind]))
        (raw / "original.png").write_bytes(png)
        tex = seamless(Image.open(io.BytesIO(png))).resize((SIZE, SIZE), Image.LANCZOS)
        tex.save(dest)
        preview = Image.new("RGB", (SIZE * 3, SIZE * 3))
        for i in range(9):
            preview.paste(tex, (i % 3 * SIZE, i // 3 * SIZE))
        preview.save(raw / "tiled.png")
        print(f"Saved {dest}; 3x3 tiling preview: {raw / 'tiled.png'}")
        if input("Regenerate? [y/N] ").strip().lower() != "y":
            break
    if kind == "wall":
        rows = {l.split()[1]: l.split()[0] for l in TABLE.read_text().splitlines() if l.strip() and not l.startswith("#")}
        ch = rows.get(dest.stem)
        if not ch:
            ch = free_char()
            with TABLE.open("a") as f:
                f.write(f"{ch} {dest.stem}\n")
        print(f"Wall material '{ch}': paint it in the editor, or use {ch} in a .map grid.")
    else:
        print(f"Pick it in the editor's floor/ceiling row, or add \"{kind} {name}\" to a .map file.")


def main():
    key = api_key()
    while True:
        create(key)
        if input("Create another texture? [y/N] ").strip().lower() != "y":
            break


if __name__ == "__main__":
    main()
