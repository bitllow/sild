#!/usr/bin/env python3
"""Write the README's consumer-smoke snippet to a Swift file, so the documented
code and the code CI compiles cannot drift.

    extract-consumer-snippet.py <README.md> <out.swift>
"""
import re
import sys

readme, out = sys.argv[1], sys.argv[2]
block = re.search(
    r"<!-- consumer-smoke:start -->\s*```swift\n(.*?)```\s*<!-- consumer-smoke:end -->",
    open(readme).read(),
    re.S,
)
if not block:
    sys.exit(f"{readme}: consumer-smoke markers not found")
open(out, "w").write(block.group(1))
