#!/usr/bin/env python3
"""Generate the Cubicle macOS app icon set."""

import os
import shutil
import subprocess
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter


CANVAS = 1024
WORK_SCALE = 3


def rounded_mask(size, radius):
    mask = Image.new("L", (size, size), 0)
    draw = ImageDraw.Draw(mask)
    draw.rounded_rectangle((0, 0, size - 1, size - 1), radius=radius, fill=255)
    return mask


def lerp(a, b, t):
    return int(a + (b - a) * t)


def vertical_gradient(size, top, bottom):
    image = Image.new("RGBA", (size, size))
    pixels = image.load()
    for y in range(size):
        t = y / max(size - 1, 1)
        color = (
            lerp(top[0], bottom[0], t),
            lerp(top[1], bottom[1], t),
            lerp(top[2], bottom[2], t),
            255,
        )
        for x in range(size):
            pixels[x, y] = color
    return image


def draw_soft_ellipse(layer, box, color, blur):
    glow = Image.new("RGBA", layer.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(glow)
    draw.ellipse(box, fill=color)
    layer.alpha_composite(glow.filter(ImageFilter.GaussianBlur(blur)))


def draw_shadow(layer, shape_drawer, blur=24, offset=(0, 14), opacity=80):
    shadow = Image.new("RGBA", layer.size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(shadow)
    shape_drawer(draw, offset, (7, 28, 51, opacity))
    layer.alpha_composite(shadow.filter(ImageFilter.GaussianBlur(blur)))


def panel(draw, points, fill, outline, width):
    draw.polygon(points, fill=fill)
    draw.line(points + [points[0]], fill=outline, width=width, joint="curve")


def scaled(points, s):
    return [(int(x * s), int(y * s)) for x, y in points]


def make_master_icon():
    size = CANVAS * WORK_SCALE
    s = WORK_SCALE
    icon = Image.new("RGBA", (size, size), (0, 0, 0, 0))

    background = vertical_gradient(
        size,
        top=(17, 84, 130),
        bottom=(32, 178, 160),
    )
    glow = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw_soft_ellipse(glow, (80 * s, -120 * s, 760 * s, 520 * s), (105, 219, 244, 120), 86 * s)
    draw_soft_ellipse(glow, (470 * s, 420 * s, 1120 * s, 1120 * s), (41, 240, 169, 108), 92 * s)
    background.alpha_composite(glow)

    bg_mask = rounded_mask(size, 228 * s)
    icon.alpha_composite(background)
    icon.putalpha(bg_mask)

    shine = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    shine_draw = ImageDraw.Draw(shine)
    shine_draw.rounded_rectangle(
        (58 * s, 50 * s, 966 * s, 956 * s),
        radius=188 * s,
        outline=(255, 255, 255, 80),
        width=4 * s,
    )
    shine_draw.arc(
        (122 * s, 84 * s, 904 * s, 850 * s),
        start=205,
        end=310,
        fill=(255, 255, 255, 56),
        width=10 * s,
    )
    icon.alpha_composite(shine)

    mark = Image.new("RGBA", (size, size), (0, 0, 0, 0))

    ring = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ring_draw = ImageDraw.Draw(ring)
    ring_draw.arc(
        (238 * s, 188 * s, 786 * s, 736 * s),
        start=210,
        end=340,
        fill=(255, 255, 255, 78),
        width=16 * s,
    )
    ring_draw.arc(
        (286 * s, 238 * s, 738 * s, 690 * s),
        start=20,
        end=184,
        fill=(70, 220, 199, 82),
        width=9 * s,
    )
    mark.alpha_composite(ring)

    draw_soft_ellipse(mark, (290 * s, 688 * s, 734 * s, 808 * s), (7, 29, 47, 58), 22 * s)

    floor_points = scaled(
        [(318, 642), (512, 568), (706, 642), (512, 758)],
        s,
    )

    def floor_shape(draw, offset, color):
        ox, oy = offset
        shifted = [(x + ox, y + oy) for x, y in floor_points]
        draw.polygon(shifted, fill=color)

    draw_shadow(mark, floor_shape, blur=30 * s, offset=(0, 28 * s), opacity=72)
    mark_draw = ImageDraw.Draw(mark)
    panel(mark_draw, floor_points, (220, 247, 255, 240), (255, 255, 255, 142), 3 * s)

    grid_color = (55, 119, 156, 48)
    for i in range(1, 4):
        t = i / 4
        left = (
            lerp(floor_points[0][0], floor_points[1][0], t),
            lerp(floor_points[0][1], floor_points[1][1], t),
        )
        right = (
            lerp(floor_points[3][0], floor_points[2][0], t),
            lerp(floor_points[3][1], floor_points[2][1], t),
        )
        mark_draw.line([left, right], fill=grid_color, width=2 * s)
        top = (
            lerp(floor_points[1][0], floor_points[2][0], t),
            lerp(floor_points[1][1], floor_points[2][1], t),
        )
        bottom = (
            lerp(floor_points[0][0], floor_points[3][0], t),
            lerp(floor_points[0][1], floor_points[3][1], t),
        )
        mark_draw.line([top, bottom], fill=grid_color, width=2 * s)

    rear_wall = scaled([(356, 382), (512, 302), (668, 382), (668, 562), (512, 626), (356, 562)], s)
    left_wall = scaled([(286, 418), (512, 302), (512, 626), (286, 682)], s)
    right_wall = scaled([(512, 302), (738, 418), (738, 682), (512, 626)], s)

    def wall_shadow_shape(points):
        def drawer(draw, offset, color):
            ox, oy = offset
            shifted = [(x + ox, y + oy) for x, y in points]
            draw.polygon(shifted, fill=color)

        return drawer

    for points in (rear_wall, left_wall, right_wall):
        draw_shadow(mark, wall_shadow_shape(points), blur=14 * s, offset=(0, 12 * s), opacity=45)

    panel(mark_draw, rear_wall, (235, 250, 255, 248), (255, 255, 255, 185), 4 * s)
    panel(mark_draw, left_wall, (242, 252, 255, 245), (255, 255, 255, 190), 4 * s)
    panel(mark_draw, right_wall, (204, 241, 249, 245), (255, 255, 255, 170), 4 * s)

    seam_color = (41, 118, 151, 70)
    mark_draw.line(scaled([(512, 302), (512, 626)], s), fill=seam_color, width=5 * s)
    mark_draw.line(scaled([(286, 418), (512, 302), (738, 418)], s), fill=seam_color, width=4 * s)
    mark_draw.line(scaled([(286, 682), (512, 626), (738, 682)], s), fill=seam_color, width=4 * s)

    desk = scaled([(370, 610), (512, 550), (654, 610), (512, 704)], s)
    panel(mark_draw, desk, (18, 69, 102, 235), (130, 228, 242, 145), 3 * s)
    screen = scaled([(450, 594), (512, 565), (574, 594), (512, 636)], s)
    panel(mark_draw, screen, (91, 228, 207, 245), (239, 255, 255, 190), 2 * s)

    def chat_bubble(x, y, w, h, fill, outline):
        rect = (x * s, y * s, (x + w) * s, (y + h) * s)
        mark_draw.rounded_rectangle(rect, radius=22 * s, fill=fill, outline=outline, width=4 * s)
        tail = scaled([(x + 36, y + h - 2), (x + 30, y + h + 30), (x + 72, y + h - 2)], s)
        mark_draw.polygon(tail, fill=fill)
        mark_draw.line([tail[1], tail[2]], fill=outline, width=4 * s)

    draw_shadow(
        mark,
        lambda draw, offset, color: draw.rounded_rectangle(
            (
                622 * s + offset[0],
                618 * s + offset[1],
                788 * s + offset[0],
                718 * s + offset[1],
            ),
            radius=34 * s,
            fill=color,
        ),
        blur=20 * s,
        offset=(0, 14 * s),
        opacity=62,
    )
    chat_bubble(642, 618, 146, 100, (244, 255, 255, 242), (255, 255, 255, 185))
    for cx, cy, r, color in [
        (684, 668, 9, (43, 129, 244, 255)),
        (715, 668, 9, (40, 198, 142, 255)),
        (746, 668, 9, (249, 198, 70, 255)),
    ]:
        mark_draw.ellipse(
            ((cx - r) * s, (cy - r) * s, (cx + r) * s, (cy + r) * s),
            fill=color,
        )

    icon.alpha_composite(mark)
    return icon.resize((CANVAS, CANVAS), Image.Resampling.LANCZOS)


def write_iconset(master, output_icns):
    output_icns = Path(output_icns)
    iconset = output_icns.with_suffix(".iconset")
    if iconset.exists():
        shutil.rmtree(iconset)
    iconset.mkdir(parents=True)

    variants = [
        ("icon_16x16.png", 16),
        ("icon_16x16@2x.png", 32),
        ("icon_32x32.png", 32),
        ("icon_32x32@2x.png", 64),
        ("icon_128x128.png", 128),
        ("icon_128x128@2x.png", 256),
        ("icon_256x256.png", 256),
        ("icon_256x256@2x.png", 512),
        ("icon_512x512.png", 512),
        ("icon_512x512@2x.png", 1024),
    ]
    for filename, size in variants:
        resized = master.resize((size, size), Image.Resampling.LANCZOS)
        resized.save(iconset / filename)

    output_icns.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["iconutil", "-c", "icns", str(iconset), "-o", str(output_icns)], check=True)


def main():
    if len(sys.argv) != 2:
        print("usage: generate-app-icon.py OUTPUT.icns", file=sys.stderr)
        return 2
    output = Path(sys.argv[1])
    master = make_master_icon()
    write_iconset(master, output)
    print(f"Generated {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
