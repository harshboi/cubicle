from pathlib import Path


def test_static_assets_are_ascii_clean():
    static_dir = Path(__file__).resolve().parents[1] / "voicenotes_app" / "static"
    for path in static_dir.iterdir():
        if path.suffix in {".html", ".js", ".css"}:
            path.read_text(encoding="ascii")

