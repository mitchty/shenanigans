#!/usr/bin/env sh
#
# Quick helper script to find what iommu group a device is in.
#shellcheck disable=SC2231
cur=''

devname() {
  vendor=${1%:*}
  product=${1#*:}
  for path in $(find /sys/ -name idVendor | rev | cut -d/ -f 2- | rev); do
    if grep -q $vendor $path/idVendor && grep -q $product $path/idProduct; then
      find $path -name 'device' | rev | cut -d / -f 2 | rev
    fi
  done
}
for dir in $(find /sys/kernel/iommu_groups/ -type l | sort -n -k5 -t/); do
  iommu=${dir#*/iommu_groups/*}
  iommu=${iommu%%/*}
  if [ "${iommu}" != "${cur}" ]; then
    printf 'iommu group %s:\n' "${iommu}"
    cur="${iommu}"
  fi

  lspci -nns "${dir##*/}"

  for usb in ${dir}/usb*/; do
    if [ -e "${usb}/busnum" ]; then
      bus=$(cat "${usb}/busnum")
      usbdev=$(lsusb -s "${bus}": \
        | awk '{gsub(/:/,"",$4); printf "%s|%s %s %s %s|", $6, $1, $2, $3, $4; for(i=7;i<=NF;i++){printf "%s ", $i}; printf "\n"}' \
        | awk -F'|' '{printf "usb:\t[%s]\t %-40s %s\n", $1, $2, $3}')
      # Get the usb /dev/name (I could do this better but ^^^ was already a hack so whatever)
      # bu=$(echo "${usbdev}" | awk '{print $2}' | tr -d \[\])
      # dn=$(devname "${bu}")
      # printf "device: %s\n" "${dn}"
      printf "%s" "${usbdev}"
    fi
  done

  for net in ${dir}/net/*; do
    if [ -e "${net}/address" ]; then
      name=$(basename "${net}")
      mac=$(cat "${net}/address")
      printf "net:\t%s\tmac %s\n" "${name}" "${mac}"
    fi
  done
done
