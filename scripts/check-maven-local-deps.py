#!/usr/bin/env python3
"""Assert every same-group dependency of a Maven-local POM was actually published.

    check-maven-local-deps.py <group> <artifact>

A KMP module publishes one artifact per target plus a root module, and a POM that
depends on the root is unsatisfiable unless that root was published too. Publishing
tasks succeed either way, so nothing else catches it.
"""
import os
import pathlib
import re
import sys
import xml.etree.ElementTree as ET

POM = "{http://maven.apache.org/POM/4.0.0}"
group, artifact = sys.argv[1], sys.argv[2]
repo = pathlib.Path(os.environ.get("HOME", "")) / ".m2" / "repository"
base = repo / group.replace(".", "/") / artifact

poms = sorted(base.glob("*/*.pom"))
if not poms:
    sys.exit(f"::error::{group}:{artifact} was not published to {repo}")

failed = []
for pom in poms:
    root = ET.parse(pom).getroot()
    for dep in root.iter(f"{POM}dependency"):
        g = dep.findtext(f"{POM}groupId", "")
        a = dep.findtext(f"{POM}artifactId", "")
        v = dep.findtext(f"{POM}version", "")
        if g != group:
            continue
        if not (repo / g.replace(".", "/") / a / v / f"{a}-{v}.pom").exists():
            failed.append(f"{pom.name} requires {g}:{a}:{v}, which was not published")

for line in failed:
    print(f"::error::{line}")
if failed:
    sys.exit(1)
print(f"{group}:{artifact}: every {group} dependency resolves in {repo}")
