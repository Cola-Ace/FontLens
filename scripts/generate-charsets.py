"""Regenerate built-in repertoires with Python 3.12: python scripts/generate-charsets.py."""

import argparse
import io
import json
from pathlib import Path
import re
import sys
from urllib.request import Request, urlopen
import zipfile


ROOT = Path(__file__).resolve().parents[1]
CACHE = ROOT / ".cache" / "charsets"
DATA = ROOT / "internal" / "coverage" / "data"
UNICODE = "https://www.unicode.org/Public/17.0.0/ucd/"
LEGACY = "https://www.unicode.org/Public/MAPPINGS/OBSOLETE/EASTASIA/"
ICU = "https://raw.githubusercontent.com/unicode-org/icu/release-76-1/icu4c/source/data/mappings/gb18030-2022.ucm"
HKSCS = "https://www.digitalpolicy.gov.hk/open_data/ccli/HKSCS2016.json"
JIS0213 = "https://x0213.org/codetable/jisx0213-2004-std.txt"


def fetch(url):
    path = CACHE / url.rsplit("/", 1)[-1]
    if not path.exists():
        request = Request(url, headers={"User-Agent": "Mozilla/5.0 FontLens-charset-generator"})
        with urlopen(request, timeout=60) as response:
            data = response.read()
        CACHE.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    return path.read_bytes()


def span(first, last):
    return set(range(first, last + 1))


def generate():
    if sys.version_info[:2] != (3, 12):
        raise RuntimeError("Use Python 3.12 for the pinned Big5 codec mapping")

    archive = zipfile.ZipFile(io.BytesIO(fetch(UNICODE + "Unihan.zip")))
    tgh = [set(), set(), set()]
    numbers = set()
    for line in archive.read("Unihan_OtherMappings.txt").decode("utf-8").splitlines():
        if "\tkTGH\t" not in line:
            continue
        codepoint, _, value = line.split("\t")
        year, number = value.split(":")
        assert year == "2013"
        number = int(number)
        assert number not in numbers and 1 <= number <= 8105
        numbers.add(number)
        tgh[0 if number <= 3500 else 1 if number <= 6500 else 2].add(int(codepoint[2:], 16))
    common = set.union(*tgh)
    assert len(numbers) == 8105

    # GB18030-2022 keeps 12 compatibility ideographs without canonical decompositions.
    compatibility = {int(c, 16) for c in "FA0E FA0F FA11 FA13 FA14 FA1F FA21 FA23 FA24 FA27 FA28 FA29".split()}
    mappings = re.findall(r"^<U([0-9A-F]+)>\s+((?:\\x[0-9A-F]{2})+)\s+\|0", fetch(ICU).decode("utf-8"), re.M)
    radicals = {int(c, 16) for c, encoded in mappings if len(encoded) == 8 and 0x2E80 <= int(c, 16) <= 0x2EFF}
    assert len(radicals) == 14
    level1 = span(0x3400, 0x4DB5) | span(0x4E00, 0x9FEF) | compatibility | radicals
    level2 = level1 | common
    assert len(common - level1) == 196
    # All 9,793 additions in the 2024 amendment belong to level 3.
    level3 = compatibility | radicals | span(0x2F00, 0x2FD5)
    for first, last in [
        (0x3400, 0x4DBF), (0x4E00, 0x9FFF), (0x20000, 0x2A6DF),
        (0x2A700, 0x2B739), (0x2B740, 0x2B81D), (0x2B820, 0x2CEA1),
        (0x2CEB0, 0x2EBE0), (0x2EBF0, 0x2EE5D),
        (0x30000, 0x3134A), (0x31350, 0x323AF),
    ]:
        level3 |= span(first, last)
    assert level1 < level2 < level3

    big5 = set()
    positions = 0
    for lead in range(0xA1, 0xFA):
        for trail in [*range(0x40, 0x7F), *range(0xA1, 0xFF)]:
            try:
                character = bytes([lead, trail]).decode("big5")
            except UnicodeDecodeError:
                continue
            assert len(character) == 1
            positions += 1
            big5.add(ord(character))
    assert positions == 13710

    hkscs_rows = json.loads(fetch(HKSCS).decode("utf-8-sig"))
    assert len(hkscs_rows) == 5033
    hkscs = set()
    for row in hkscs_rows:
        codepoint = int(row["codepoint"], 16)
        assert row["hkscs-ver"] <= 2016 and row["char"] == chr(codepoint)
        hkscs.add(codepoint)
    # The official JSON includes the approved 2018 mapping correction.
    assert 0x270F0 in hkscs and 0x2F9B2 not in hkscs

    jis0208 = {
        int(c, 16) for c in re.findall(
            r"^0x[0-9A-F]+\s+0x[0-9A-F]+\s+0x([0-9A-F]+)",
            fetch(LEGACY + "JIS/JIS0208.TXT").decode("utf-8"), re.M,
        )
    }
    jis_mappings = re.findall(r"^[34]-[0-9A-F]+\s+U\+([0-9A-F+]+)", fetch(JIS0213).decode("utf-8"), re.M)
    assert len(jis_mappings) == 11233 and sum("+" in row for row in jis_mappings) == 25
    jis0213 = {int(c, 16) for row in jis_mappings for c in row.split("+")}

    ks_mappings = re.findall(r"^0x([0-9A-F]+)\s+0x([0-9A-F]+)", fetch(LEGACY + "KSC/KSC5601.TXT").decode("utf-8"), re.M)
    ks1001 = {int(c, 16) for encoded, c in ks_mappings if 0xA1 <= int(encoded, 16) >> 8 <= 0xFE and 0xA1 <= int(encoded, 16) & 255 <= 0xFE}
    assert len(ks1001 & span(0xAC00, 0xD7A3)) == 2350

    return [
        ("gb18030-2022-1", level1, 27584),
        ("gb18030-2022-2", level2, 27780),
        ("gb18030-2022-3", level3, 97908),
        ("tgh-2013-1", tgh[0], 3500),
        ("tgh-2013-2", tgh[1], 3000),
        ("tgh-2013-3", tgh[2], 1605),
        ("tgh-2013", common, 8105),
        ("big5", big5, 13706),
        ("hkscs-2016", hkscs, 5033),
        ("jis0208-1990", jis0208, 6879),
        ("jis0213-2004", jis0213, 11209),
        ("ks1001-1992", ks1001, 8224),
        ("hangul-syllables", span(0xAC00, 0xD7A3), 11172),
    ]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="compare with committed data without writing it")
    args = parser.parse_args()
    repertoires = generate()
    for name, members, expected in repertoires:
        assert len(members) == expected, (name, len(members), expected)
        assert all(0 <= c <= 0x10FFFF and not 0xD800 <= c <= 0xDFFF for c in members)
    for name, members, expected in repertoires:
        content = "".join(map(chr, sorted(members))).encode("utf-8")
        path = DATA / (name + ".txt")
        if args.check:
            assert path.read_bytes() == content, f"Regenerate {path.name}"
        else:
            path.write_bytes(content)
        print(f"{path.name}: {expected}")


if __name__ == "__main__":
    main()
