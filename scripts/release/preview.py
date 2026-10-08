#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Build unpublished review archives; published releases use GoReleaser."""
import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
from notices import ROOT, notices

def main():
    version=(ROOT/"VERSION").read_text().strip()
    destination=ROOT/".artifacts"/f"songstead-{version}-preview"
    destination.parent.mkdir(parents=True,exist_ok=True)
    if destination.exists(): raise FileExistsError(destination)
    temporary_output=tempfile.TemporaryDirectory(prefix="release-preview-",dir=destination.parent)
    staging=Path(temporary_output.name)
    final_destination=destination
    destination=staging
    notice=notices().encode()
    for arch in ("amd64","arm64"):
        prefix=f"songstead_{version}_linux_{arch}"
        with tempfile.TemporaryDirectory() as temporary:
            binary=Path(temporary)/"songstead"
            subprocess.run(["go","build","-trimpath","-ldflags",f"-X main.version={version}","-o",str(binary),"./cmd/songstead"],cwd=ROOT,
                env={**os.environ,"CGO_ENABLED":"0","GOOS":"linux","GOARCH":arch},check=True)
            archive=destination/f"{prefix}.tar.gz"
            with tarfile.open(archive,"w:gz") as tar:
                tar.add(binary,arcname=f"{prefix}/songstead")
                for path in (ROOT/"LICENSE",ROOT/"README.md",ROOT/"CHANGELOG.md",ROOT/"THIRD_PARTY.md",ROOT/"docs",ROOT/"contrib"):
                    tar.add(path,arcname=f"{prefix}/{path.name}")
                entry=tarfile.TarInfo(f"{prefix}/THIRD_PARTY_NOTICES.txt");entry.size=len(notice);tar.addfile(entry,io.BytesIO(notice))
    lines=[f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}" for p in sorted(destination.glob("*.tar.gz"))]
    (destination/f"songstead_{version}_checksums.txt").write_text("\n".join(lines)+"\n")
    (destination/"UNPUBLISHED.txt").write_text("Local review builds using the sibling Comfylib workspace. Not published release artifacts. Publish Comfylib, resolve go.sum, and run the release workflow before distribution.\n")
    os.rename(staging,final_destination)
    temporary_output.cleanup()
    print(final_destination)

if __name__=="__main__":main()
