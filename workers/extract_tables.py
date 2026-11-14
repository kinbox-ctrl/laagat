"""Extract table rows from a text-based PDF and print them as JSON.

Usage: extract_tables.py <file.pdf>

Output: a JSON array of rows; each row is an array of cell strings. The Go
service looks for the BOQ header row in this grid, so no BOQ-specific logic
lives here. Scanned (image-only) PDFs produce no rows and exit with an error:
those documents are entered manually by an analyst in V1.
"""
import json
import sys

import pdfplumber


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: extract_tables.py <file.pdf>", file=sys.stderr)
        return 2
    rows = []
    with pdfplumber.open(sys.argv[1]) as pdf:
        for page in pdf.pages:
            for table in page.extract_tables():
                for row in table:
                    cells = [(c or "").replace("\n", " ").strip() for c in row]
                    if any(cells):
                        rows.append(cells)
    if not rows:
        print("no tables found; the PDF may be a scan or the BOQ may not be laid out as a ruled table", file=sys.stderr)
        return 1
    json.dump(rows, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
