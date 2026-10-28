#!/usr/bin/env python3
"""Build the xngxltuiplayer logo and Windows icon from a transparent PNG mark.

Usage:
    python tools/make_icon.py path/to/mark.png assets

Pillow is only needed when regenerating the committed assets.
"""

from __future__ import annotations

import struct
import sys
from io import BytesIO
from pathlib import Path

from PIL import Image, ImageDraw

ORANGE = "#FF6B00"
MASTER = 1024
CIRCLE_MARGIN = 22
LOGO_MAX_W = 660
LOGO_MAX_H = 730
ICON_SIZES = [16, 24, 32, 48, 64, 128, 256]
SMALL_SIZES = {16, 24, 32}


def build_master(mark_path: Path) -> Image.Image:
    mark = Image.open(mark_path).convert("RGBA")
    bbox = mark.getchannel("A").getbbox()
    if bbox is None:
        raise SystemExit("source image has no visible pixels")
    mark = mark.crop(bbox)

    canvas = Image.new("RGBA", (MASTER, MASTER), (0, 0, 0, 0))
    draw = ImageDraw.Draw(canvas)
    draw.ellipse(
        (CIRCLE_MARGIN, CIRCLE_MARGIN, MASTER - CIRCLE_MARGIN, MASTER - CIRCLE_MARGIN),
        fill=ORANGE,
    )

    scale = min(LOGO_MAX_W / mark.width, LOGO_MAX_H / mark.height)
    logo = mark.resize(
        (max(1, round(mark.width * scale)), max(1, round(mark.height * scale))),
        Image.Resampling.LANCZOS,
    )
    left = (MASTER - logo.width) // 2
    top = (MASTER - logo.height) // 2
    canvas.alpha_composite(logo, (left, top))
    return canvas


def build_small(size: int) -> Image.Image:
    """A simplified pause mark keeps 16-32 px icons readable."""
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    margin = max(0, round(size * 0.02))
    draw.ellipse((margin, margin, size - 1 - margin, size - 1 - margin), fill=ORANGE)

    center = size / 2
    if size > 20:
        ring_r = size * 0.29
        ring_w = max(1, round(size * 0.085))
        draw.ellipse(
            (center - ring_r, center - ring_r, center + ring_r, center + ring_r),
            outline="black",
            width=ring_w,
        )

    bar_w = max(2, round(size * 0.12 if size <= 20 else size * 0.10))
    bar_h = max(5, round(size * 0.34 if size <= 20 else size * 0.30))
    gap = max(2, round(size * 0.08))
    top = round(center - bar_h / 2)
    bottom = top + bar_h
    left = round(center - gap / 2 - bar_w)
    right = round(center + gap / 2)
    draw.rectangle((left, top, left + bar_w - 1, bottom), fill="black")
    draw.rectangle((right, top, right + bar_w - 1, bottom), fill="black")
    return img


def ico_bytes(images: list[Image.Image]) -> bytes:
    """Pack per-size images as 32-bit BMP/DIB entries (rsrc-compatible)."""
    entries: list[bytes] = []
    payloads: list[bytes] = []
    for img in images:
        buf = BytesIO()
        img.save(
            buf,
            format="ICO",
            sizes=[(img.width, img.height)],
            bitmap_format="bmp",
        )
        data = buf.getvalue()
        count = struct.unpack_from("<H", data, 4)[0]
        if count != 1:
            raise SystemExit("Pillow produced an unexpected ICO")
        width, height, colors, reserved, planes, bits, size, offset = struct.unpack_from(
            "<BBBBHHII", data, 6
        )
        payload = data[offset : offset + size]
        entries.append(
            struct.pack(
                "<BBBBHHII",
                width,
                height,
                colors,
                reserved,
                planes,
                bits,
                size,
                0,  # patched below
            )
        )
        payloads.append(payload)

    header = struct.pack("<HHH", 0, 1, len(entries))
    offset = 6 + 16 * len(entries)
    out = bytearray(header)
    for entry, payload in zip(entries, payloads):
        patched = entry[:12] + struct.pack("<I", offset)
        out.extend(patched)
        offset += len(payload)
    for payload in payloads:
        out.extend(payload)
    return bytes(out)


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    source = Path(sys.argv[1])
    out_dir = Path(sys.argv[2])
    out_dir.mkdir(parents=True, exist_ok=True)

    master = build_master(source)
    master.save(out_dir / "logo.png", optimize=True)
    master.resize((256, 256), Image.Resampling.LANCZOS).save(
        out_dir / "logo-256.png", optimize=True
    )

    images = []
    for size in ICON_SIZES:
        if size in SMALL_SIZES:
            images.append(build_small(size))
        else:
            images.append(master.resize((size, size), Image.Resampling.LANCZOS))
    (out_dir / "xngxltuiplayer.ico").write_bytes(ico_bytes(images))


if __name__ == "__main__":
    main()
