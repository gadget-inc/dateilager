#!/usr/bin/env bash

# This script is used to resize the base LV created by cached to a given amount of free space.

set -euo pipefail

format_bytes() {
    numfmt --to=iec-i --suffix=B --format="%.2f" "$1"
}

if [[ $EUID -ne 0 ]]; then
    echo "This script must be run as root (use sudo)"
    exit 1
fi

if [[ $# -ne 1 ]]; then
    echo "Usage: sudo $0 <free space in GiB>"
    echo
    echo "Example:"
    echo "  # Shrink to 8 GiB of free space"
    echo "  sudo $0 8"
    exit 1
fi

DESIRED_FREE_GIB="$1"
VG=vg_dl_cache
LV=base
DEV=/dev/mapper/$VG-$LV
MNT=/mnt/$VG/$LV

# Get current LV size in bytes
current_bytes=$(lvs -o lv_size --units b --noheading --nosuffix "$VG/$LV" | tr -d '[:space:]')

# Get used bytes from filesystem's perspective
mkdir -p "$MNT"
mount "$DEV" "$MNT"
used_bytes=$(df -B1 --output=used "$MNT" | tail -1)
umount "$MNT"

# Get device's block size
blocksize=$(dumpe2fs -h "$DEV" 2>/dev/null | awk -F: '/Block size/ {gsub(/ /,""); print $2}')

# Get minimum shrink estimate
min_blocks=$(resize2fs -P "$DEV" 2>/dev/null | awk '{print $NF}')
min_bytes=$((min_blocks * blocksize))

# Calculate target bytes
desired_free_bytes=$((DESIRED_FREE_GIB * 1024**3))
target_bytes=$((used_bytes + desired_free_bytes))
if (( target_bytes < min_bytes )); then
    echo "Cannot shrink below minimum. Using minimum instead of target."
    target_bytes=$min_bytes
fi

# Get volume group's extent size
pe_kb=$(vgdisplay -c "$VG" | cut -d: -f13)
pe_bytes=$((pe_kb * 1024))

# Round target to nearest extent
target_bytes=$(( (target_bytes + pe_bytes - 1) / pe_bytes * pe_bytes ))

echo "Used:    $(format_bytes "$used_bytes")"
echo "Current: $(format_bytes "$current_bytes")"
echo "Minimum: $(format_bytes "$min_bytes")"
echo "Target:  $(format_bytes "$target_bytes")"
echo
read -p "Resize? (y/N): " confirm
if [ "$confirm" != "y" ]; then
    echo "Aborting."
    exit 1
fi

# Check filesystem integrity
e2fsck -f "$DEV"

# Convert target to extents
target_extents=$((target_bytes / pe_bytes))

# Resize LV
lvresize -r -l "$target_extents" "$VG/$LV"
