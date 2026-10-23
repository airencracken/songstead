#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Check native archives, checksums, and tracked source documentation."""
from pathlib import Path
import hashlib
import subprocess
import tarfile
import tempfile
from release import ROOT, stage


def main():
    dist = ROOT / 'dist'
    manifests = list(dist.glob('songstead_*_checksums.txt'))
    if len(manifests) != 1:
        raise ValueError('expected one checksum manifest')
    for line in manifests[0].read_text().splitlines():
        digest, name = line.split(maxsplit=1)
        if Path(name).name != name:
            raise ValueError('unsafe checksum path')
        if hashlib.sha256((dist / name).read_bytes()).hexdigest() != digest:
            raise ValueError('artifact checksum mismatch')
    with tempfile.TemporaryDirectory() as temporary:
        for arch in ('amd64', 'arm64'):
            stage(dist, Path(temporary) / arch, 'songstead', arch)
    sources = list(dist.glob('songstead_*_source.tar.gz'))
    if len(sources) != 1:
        raise ValueError('expected one source archive')
    paths = subprocess.run(['git', 'ls-files', '-z', '--', 'docs', 'contrib', 'scripts', 'cmd', 'internal', 'README.md', 'Dockerfile', 'Dockerfile.release', 'docker-compose.yml', 'docker-compose.build.yml', '.dockerignore'],
                           cwd=ROOT, check=True, capture_output=True).stdout.decode().split('\0')
    with tarfile.open(sources[0]) as archive:
        prefix = sources[0].name.removesuffix('.tar.gz')
        for path in filter(None, paths):
            member = archive.getmember(prefix + '/' + path)
            if not member.isfile() or archive.extractfile(member).read() != (ROOT / path).read_bytes():
                raise ValueError('source archive differs: ' + path)
    print('Songstead release archives and documentation verified')


if __name__ == '__main__':
    main()
