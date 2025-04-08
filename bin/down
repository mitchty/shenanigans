#!/usr/bin/env sh
#-*-mode: Shell-script; coding: utf-8;-*-
# SPDX-License-Identifier: BlueOak-1.0.0
# Description: Nuke libvirt pulumi setup from orbit
# Got lazy with these being in my history so script it is.
_base=$(basename "$0")
_dir=$(cd -P -- "$(dirname -- "$(command -v -- "$0")")" && pwd -P || exit 126)
export _base _dir
set "${SETOPTS:--u}"

sudo pkill -KILL libvirtd
sleep 1
pulumi destroy -y
sudo libvirt-wipe.sh
sleep 1
pulumi refresh -yf
sleep 1
pulumi cancel -y
