#!/usr/bin/env sh

# This will try to wipe everything, its like a nuke to clean up
# libvirt turds that get left behind somehow. Run it at your own
# peril. The intent I operate under is everything can be recreated,
# aka thats kinda the point of this repo. So hitting a giant reset
# button is oki doki.

# Past nuking/moving /var/lib/libvirt this is the next best thing.

# Nuke all vm's+disks attached to them first
for vm in $(virsh list --name --all); do
  virsh destroy "${vm}"

  for disk in $(virsh -q domblklist "${vm}" | awk '{print $2}'); do
    pool=$(virsh vol-pool "${disk}")
    name=$(virsh vol-info "${disk}" | awk '$1 == "Name:" {print $2}')

    virsh vol-delete "${name}" "${pool}"
  done

  for disk in $(virsh -q domblklist --inactive "${vm}" | awk '{print $2}'); do
    pool=$(virsh vol-pool "${disk}")
    name=$(virsh vol-info "${disk}" | awk '$1 == "Name:" {print $2}')

    virsh vol-delete "${name}" "${pool}"
  done

  virsh undefine "${vm}"
done

# Now networks that aren't default (leave that one alone)
for net in $(virsh net-list --all --name | awk '!/default/'); do
  virsh net-destroy "${net}"
  virsh net-undefine "${net}"
done

# Now nuke the pools also leave default alone
for pool in $(virsh pool-list --all --name | awk '!/default/'); do
  virsh pool-destroy "${pool}"
  virsh pool-undefine "${pool}"
done
