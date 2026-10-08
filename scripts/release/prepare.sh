#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Never resolve a release using an unpublished sibling workspace.
export GOWORK=off
go mod download || exit 1
go mod verify || exit 1
git diff --exit-code -- go.mod go.sum || exit 1
mkdir -p .release || exit 1
python3 scripts/release/notices.py > .release/THIRD_PARTY_NOTICES.txt || exit 1
