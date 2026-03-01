#!/usr/bin/env python3
"""Fast PDF regression calibration/testing for patent-screen reports.

Usage:
  python3 scripts/pdf_regression.py calibrate
  python3 scripts/pdf_regression.py test
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path

try:
    from PIL import Image
except Exception as exc:  # pragma: no cover
    raise SystemExit(
        "Missing dependency: Pillow (python3-pil). Install it and rerun."
    ) from exc


ROOT = Path(__file__).resolve().parents[1]
CONFIG_PATH = ROOT / "tests" / "pdf_regression" / "calibration.json"


def require_tool(name: str) -> None:
    if shutil.which(name) is None:
        raise SystemExit(f"Missing required tool: {name}")


def load_config() -> dict:
    return json.loads(CONFIG_PATH.read_text(encoding="utf-8"))


def save_config(cfg: dict) -> None:
    CONFIG_PATH.parent.mkdir(parents=True, exist_ok=True)
    CONFIG_PATH.write_text(json.dumps(cfg, indent=2) + "\n", encoding="utf-8")


def generate_pdf(cfg: dict, out_pdf: Path) -> None:
    fixture = ROOT / cfg["fixture"]
    endpoint = cfg["endpoint"]
    body = fixture.read_bytes()
    req = urllib.request.Request(
        endpoint,
        data=body,
        method="POST",
        headers={"Content-Type": "text/plain; charset=utf-8"},
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            payload = resp.read()
            status = resp.getcode()
            ctype = resp.headers.get("Content-Type", "")
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        raise SystemExit(f"PDF endpoint HTTP {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise SystemExit(f"PDF endpoint error: {e}") from e
    if status != 200 or "application/pdf" not in ctype:
        raise SystemExit(f"Unexpected PDF response status={status} content_type={ctype}")
    out_pdf.write_bytes(payload)


def extract_page_png(pdf: Path, page: int, out_png: Path) -> None:
    prefix = out_png.with_suffix("")
    cmd = [
        "pdftoppm",
        "-f",
        str(page),
        "-singlefile",
        "-png",
        str(pdf),
        str(prefix),
    ]
    subprocess.run(cmd, check=True)


def extract_page_text(pdf: Path, page: int) -> str:
    cmd = ["pdftotext", "-f", str(page), "-l", str(page), str(pdf), "-"]
    out = subprocess.check_output(cmd, text=True)
    return out


def diff_percent(a_png: Path, b_png: Path, channel_threshold: int = 15) -> float:
    a = Image.open(a_png).convert("RGB")
    b = Image.open(b_png).convert("RGB")
    if a.size != b.size:
        return 100.0
    a_pixels = a.load()
    b_pixels = b.load()
    width, height = a.size
    total = width * height
    changed = 0
    for y in range(height):
        for x in range(width):
            p1 = a_pixels[x, y]
            p2 = b_pixels[x, y]
            if max(abs(p1[0] - p2[0]), abs(p1[1] - p2[1]), abs(p1[2] - p2[2])) > channel_threshold:
                changed += 1
    return 100.0 * changed / total


def assert_text_invariants(cfg: dict, pdf: Path) -> dict:
    checks = cfg["text_invariants"]
    p1 = extract_page_text(pdf, 1)
    p2 = extract_page_text(pdf, 2)
    result = {
        "page1_footer_present": checks["page1_footer_contains"] in p1,
        "page2_footer_present": checks["page2_footer_contains"] in p2,
        "page1_no_how_this_report_works": checks["must_start_on_page2"] not in p1,
        "page2_has_how_this_report_works": checks["must_start_on_page2"] in p2,
    }
    failed = [k for k, v in result.items() if not v]
    if failed:
        raise SystemExit(f"Text invariant failure(s): {', '.join(failed)}")
    return result


def run_calibrate(cfg: dict) -> int:
    baseline_dir = ROOT / cfg["baseline_dir"]
    baseline_dir.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="pdf-cal-") as td:
        tdp = Path(td)
        pdf = tdp / "current.pdf"
        generate_pdf(cfg, pdf)
        text_flags = assert_text_invariants(cfg, pdf)

        for page in cfg["pages"]:
            out_png = baseline_dir / f"page_{page}.png"
            extract_page_png(pdf, page, out_png)

        cfg["calibration"] = {
            "generated_at_utc": dt.datetime.utcnow().replace(microsecond=0).isoformat() + "Z",
            "text_invariants": text_flags,
            "notes": "Baseline regenerated from current server-side PDF renderer.",
        }
        save_config(cfg)
    print(f"Calibration baseline updated in {baseline_dir}")
    return 0


def run_test(cfg: dict) -> int:
    baseline_dir = ROOT / cfg["baseline_dir"]
    if not baseline_dir.exists():
        raise SystemExit(f"Baseline directory missing: {baseline_dir}. Run calibrate first.")

    with tempfile.TemporaryDirectory(prefix="pdf-test-") as td:
        tdp = Path(td)
        pdf = tdp / "current.pdf"
        generate_pdf(cfg, pdf)
        assert_text_invariants(cfg, pdf)

        max_allowed = float(cfg["thresholds"]["max_diff_pct_per_page"])
        channel_threshold = int(cfg["thresholds"].get("pixel_channel_threshold", 15))
        failures = []
        summary = {}

        for page in cfg["pages"]:
            cur_png = tdp / f"page_{page}.png"
            base_png = baseline_dir / f"page_{page}.png"
            if not base_png.exists():
                failures.append(f"missing baseline image: {base_png}")
                continue
            extract_page_png(pdf, page, cur_png)
            pct = diff_percent(base_png, cur_png, channel_threshold=channel_threshold)
            summary[f"page_{page}_diff_pct"] = round(pct, 4)
            if pct > max_allowed:
                failures.append(f"page {page} diff {pct:.3f}% exceeds {max_allowed:.3f}%")

        print(json.dumps(summary, indent=2))
        if failures:
            raise SystemExit("PDF regression failed: " + "; ".join(failures))
    print("PDF regression passed.")
    return 0


def main() -> int:
    for tool in ("pdftotext", "pdftoppm"):
        require_tool(tool)

    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("calibrate", "test"))
    args = parser.parse_args()
    cfg = load_config()
    if args.mode == "calibrate":
        return run_calibrate(cfg)
    return run_test(cfg)


if __name__ == "__main__":
    raise SystemExit(main())
