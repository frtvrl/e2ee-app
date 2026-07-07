#!/usr/bin/env python3
"""tools/lib/scanner.py — comment-strip + token match helper for ci-tools-pin-check.

Sprint 7 / STRIDE-8-03. Used by both the bash (.sh) and PowerShell
(.ps1) twins of ci-tools-pin-check. The twins shell out to this script
rather than embedding Python via heredocs (which PowerShell 5.1 mangles).

Two sub-commands:

  strip <input> <output>
      Strip comments from <input>, write to <output>. Supports shell,
      C, PowerShell, YAML, XML, HTML comment styles.

  match <input>
      Read <input> line-by-line, strip comments, tokenize, and emit
      any token matching the scanned-binary list to stdout (one match
      per line, format: "<line_no>|<binary>"). Comments and quoted
      strings are NOT stripped (treating string literals as code is the
      safer default).

The scanner is best-effort, not a full parser. False positives are easy
to suppress via `# tools-pin: skip`; false negatives would defeat the
gate, so the scanner errs on the side of caution.

Usage:
  python3 tools/lib/scanner.py strip <input> <output>
  python3 tools/lib/scanner.py match <input>
"""

from __future__ import annotations

import re
import sys
from pathlib import Path


SCANNED_BINARIES = {
    'jq', 'curl', 'wget', 'openssl', 'base64', 'sha256sum',
    'apt-get', 'apt', 'brew', 'pip', 'pip3', 'npm',
}

# Tokenize on whitespace + shell metacharacters + path separators.
_TOKEN_SPLIT_RE = re.compile(r'[\s|&;<>()"\`]+')


def strip_comments_line(line: str, in_block_c: bool, in_block_pwsh: bool,
                        in_block_xml: bool) -> tuple[str, bool, bool, bool]:
    """Strip comments from one line. Returns (new_line, in_c, in_pwsh, in_xml)."""
    out: list[str] = []
    i = 0
    n = len(line)
    while i < n:
        if not in_block_c and not in_block_pwsh and not in_block_xml:
            if line[i:i+2] == '/*':
                in_block_c = True
                i += 2
                continue
            if line[i:i+2] == '<#':
                in_block_pwsh = True
                i += 2
                continue
            if line[i:i+4] == '<!--':
                in_block_xml = True
                i += 4
                continue
            if i == 0:
                stripped = re.match(r'^(\s*)#', line)
                if stripped:
                    out.append(stripped.group(1))
                    break
                stripped = re.match(r'^(\s*)//', line)
                if stripped:
                    out.append(stripped.group(1))
                    break
            out.append(line[i])
            i += 1
        elif in_block_c:
            if line[i:i+2] == '*/':
                in_block_c = False
                i += 2
            else:
                i += 1
        elif in_block_pwsh:
            if line[i:i+2] == '#>':
                in_block_pwsh = False
                i += 2
            else:
                i += 1
        elif in_block_xml:
            if line[i:i+3] == '-->':
                in_block_xml = False
                i += 3
            else:
                i += 1
    return ''.join(out), in_block_c, in_block_pwsh, in_block_xml


def strip_file(in_path: Path, out_path: Path) -> int:
    text = in_path.read_text(encoding='utf-8', errors='replace')
    in_block_c = False
    in_block_pwsh = False
    in_block_xml = False
    out_lines: list[str] = []
    for line in text.split('\n'):
        new_line, in_block_c, in_block_pwsh, in_block_xml = strip_comments_line(
            line, in_block_c, in_block_pwsh, in_block_xml,
        )
        out_lines.append(new_line)
    out_path.write_text('\n'.join(out_lines), encoding='utf-8')
    return len(out_lines)


def match_file(in_path: Path) -> list[tuple[int, str]]:
    """Return list of (line_no, binary) for every scanned-binary match."""
    text = in_path.read_text(encoding='utf-8', errors='replace')
    in_block_c = False
    in_block_pwsh = False
    in_block_xml = False
    out: list[tuple[int, str]] = []
    for lineno, line in enumerate(text.split('\n'), start=1):
        stripped, in_block_c, in_block_pwsh, in_block_xml = strip_comments_line(
            line, in_block_c, in_block_pwsh, in_block_xml,
        )
        if not stripped.strip():
            continue
        for tok in _TOKEN_SPLIT_RE.split(stripped):
            if not tok:
                continue
            # Strip leading path components: /usr/bin/curl -> curl
            base = tok.rsplit('/', 1)[-1]
            if base in SCANNED_BINARIES:
                out.append((lineno, base))
    return out


def main() -> int:
    if len(sys.argv) < 2:
        sys.stderr.write("Usage: scanner.py {strip|match} ...\n")
        return 2
    cmd = sys.argv[1]
    if cmd == 'strip':
        if len(sys.argv) != 4:
            sys.stderr.write("Usage: scanner.py strip <input> <output>\n")
            return 2
        in_path = Path(sys.argv[2])
        out_path = Path(sys.argv[3])
        if not in_path.is_file():
            sys.stderr.write(f"ERROR: {in_path} not found\n")
            return 1
        n = strip_file(in_path, out_path)
        sys.stderr.write(f"stripped {in_path} -> {out_path} ({n} lines)\n")
        return 0
    elif cmd == 'match':
        if len(sys.argv) != 3:
            sys.stderr.write("Usage: scanner.py match <input>\n")
            return 2
        in_path = Path(sys.argv[2])
        if not in_path.is_file():
            sys.stderr.write(f"ERROR: {in_path} not found\n")
            return 1
        # Write directly to stdout in binary mode to avoid Windows
        # text-mode \n -> \r\n translation that would corrupt the
        # `lineno|binary` line format the bash/PowerShell twins
        # parse.
        with open(sys.stdout.fileno(), mode='wb', closefd=False) as raw:
            for lineno, binary in match_file(in_path):
                raw.write(f"{lineno}|{binary}\n".encode('utf-8'))
        return 0
    else:
        sys.stderr.write(f"Unknown sub-command: {cmd}\n")
        return 2


if __name__ == '__main__':
    sys.exit(main())