#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Check coordinated security regressions in disposable sibling worktrees.

This is an optional development check, not a runtime or deployment dependency.
Every target has to pass before it is mutated; compilation failures do not count.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT.parent
CASES=[
 ("comfylib","reference/reference.go",'u.User != nil','false','./reference','^TestAdversarialURLs$'),
 ("witmoot","internal/imvault/albums.go",'a.Visibility != "public"','false','./internal/imvault','^TestPrivateMalformedAndUnavailableAlbumPreviews$'),
 ("imvault","internal/store/discussions.go", "a.visibility = 'public'", "1=1",'./internal/web','^TestAlbumPreviewAuthenticationOwnershipAndPrivacy$'),
]

def main():
 for app,file,before,after,package,test in CASES:
  with tempfile.TemporaryDirectory(prefix='songstead-companion-mutation-') as temporary:
   base=Path(temporary)
   ignore=shutil.ignore_patterns('.git','go.work','go.work.sum','bin','data','dist','.release','.artifacts','node_modules','__pycache__')
   for name in ({app,'comfylib'}):shutil.copytree(BASE/name,base/name,ignore=ignore)
   cwd=base/app
   env={**os.environ,'GOPROXY':'off','GOSUMDB':'off','GOWORK':'off'}
   if app!='comfylib':
    workspace=base/'go.work';workspace.write_text(f'go 1.26.0\nuse (\n ./{app}\n ./comfylib\n)\nreplace github.com/airencracken/comfylib v0.1.2 => ./comfylib\n');env['GOWORK']=str(workspace)
   command=['go','test','-count=1','-run',test,package]
   baseline=subprocess.run(command,cwd=cwd,env=env,text=True,capture_output=True,timeout=120)
   if baseline.returncode or 'no tests to run' in baseline.stdout:raise RuntimeError(f'{app} baseline failed: {baseline.stdout}\n{baseline.stderr}')
   path=cwd/file;text=path.read_text()
   if text.count(before)!=1:raise ValueError(f'{app}: mutation anchor is ambiguous')
   path.write_text(text.replace(before,after,1))
   result=subprocess.run(command,cwd=cwd,env=env,text=True,capture_output=True,timeout=120)
   if result.returncode==0 or '--- FAIL:' not in result.stdout:raise RuntimeError(f'{app}: mutation survived or failed compilation: {result.stdout}\n{result.stderr}')
   print(f'PASS: {app} {test}',flush=True)

if __name__=='__main__':main()
