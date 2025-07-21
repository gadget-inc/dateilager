#!/bin/bash

# LVM Cleanup Script
# This script safely unmounts and removes all LVM logical volumes and volume groups

set -uo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Function to check if LVM tools are available
check_lvm_tools() {
    if ! command -v lvs &> /dev/null; then
        print_error "LVM tools not found. Please install lvm2 package."
        exit 1
    fi
}

# Function to get all mounted logical volumes
get_mounted_lvs() {
    lvs --noheadings --options lv_path,lv_name,vg_name 2>/dev/null | awk '{print $1}' | grep -v "^$" || true
}

# Function to get all volume groups
get_vgs() {
    vgs --noheadings --options vg_name 2>/dev/null | awk '{print $1}' | grep -v "^$" || true
}

# Function to unmount logical volume
unmount_lv() {
    local lv_path="$1"
    local mount_point=""
    
    # Find mount point for this LV
    mount_point=$(mount | grep "$lv_path" | awk '{print $3}' | head -1)
    
    if [[ -n "$mount_point" ]]; then
        print_status "Unmounting $lv_path from $mount_point"
        if umount "$lv_path"; then
            print_success "Successfully unmounted $lv_path"
        else
            print_warning "Failed to unmount $lv_path, trying force unmount"
            if umount -f "$lv_path"; then
                print_success "Successfully force unmounted $lv_path"
            else
                print_error "Failed to force unmount $lv_path"
                return 1
            fi
        fi
    else
        print_status "No mount point found for $lv_path"
    fi
}

# Function to remove logical volume
remove_lv() {
    local lv_path="$1"
    local lv_name=$(echo "$lv_path" | sed 's|/dev/||' | sed 's|/| |' | awk '{print $2}')
    local vg_name=$(echo "$lv_path" | sed 's|/dev/||' | sed 's|/| |' | awk '{print $1}')
    
    print_status "Removing logical volume $lv_name from volume group $vg_name"
    if lvremove -f "$lv_path"; then
        print_success "Successfully removed logical volume $lv_name"
    else
        print_error "Failed to remove logical volume $lv_name"
        return 1
    fi
}

# Function to remove volume group
remove_vg() {
    local vg_name="$1"
    
    print_status "Removing volume group $vg_name"
    if vgremove -f "$vg_name"; then
        print_success "Successfully removed volume group $vg_name"
    else
        print_error "Failed to remove volume group $vg_name"
        return 1
    fi
}

# Function to deactivate volume group
deactivate_vg() {
    local vg_name="$1"
    
    print_status "Deactivating volume group $vg_name"
    if vgchange -an "$vg_name"; then
        print_success "Successfully deactivated volume group $vg_name"
    else
        print_warning "Failed to deactivate volume group $vg_name"
    fi
}

# Main cleanup function
cleanup_lvm() {
    print_status "Starting LVM cleanup process..."
    
    # Get all logical volumes
    local lvs=($(get_mounted_lvs))
    local vgs=($(get_vgs))
    
    if [[ ${#lvs[@]} -eq 0 ]] && [[ ${#vgs[@]} -eq 0 ]]; then
        print_status "No LVM logical volumes or volume groups found"
        return 0
    fi
    
    # Step 1: Unmount all logical volumes
    print_status "Step 1: Unmounting logical volumes..."
    for lv in "${lvs[@]}"; do
        if [[ -n "$lv" ]]; then
            unmount_lv "$lv"
        fi
    done
    
    # Step 2: Remove all logical volumes
    print_status "Step 2: Removing logical volumes..."
    for lv in "${lvs[@]}"; do
        if [[ -n "$lv" ]]; then
            remove_lv "$lv"
        fi
    done
    
    # Step 3: Deactivate all volume groups
    print_status "Step 3: Deactivating volume groups..."
    for vg in "${vgs[@]}"; do
        if [[ -n "$vg" ]]; then
            deactivate_vg "$vg"
        fi
    done
    
    # Step 4: Remove all volume groups
    print_status "Step 4: Removing volume groups..."
    for vg in "${vgs[@]}"; do
        if [[ -n "$vg" ]]; then
            remove_vg "$vg"
        fi
    done
    
    print_success "LVM cleanup completed successfully!"
}

# Function to show current LVM status
show_status() {
    print_status "Current LVM status:"
    echo
    
    print_status "Logical Volumes:"
    if lvs 2>/dev/null; then
        echo
    else
        print_status "No logical volumes found"
        echo
    fi
    
    print_status "Volume Groups:"
    if vgs 2>/dev/null; then
        echo
    else
        print_status "No volume groups found"
        echo
    fi
    
    print_status "Mounted LVM devices:"
    mount | grep -E "(lvm|dm-)" || print_status "No LVM devices currently mounted"
    echo
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Options:"
    echo "  -h, --help     Show this help message"
    echo "  -s, --status   Show current LVM status without cleanup"
    echo "  -y, --yes      Skip confirmation prompt"
    echo
    echo "This script will:"
    echo "  1. Unmount all mounted LVM logical volumes"
    echo "  2. Remove all logical volumes"
    echo "  3. Deactivate all volume groups"
    echo "  4. Remove all volume groups"
    echo
    echo "WARNING: This will permanently delete all LVM data!"
}

# Main script
main() {
    local show_status_only=false
    local skip_confirmation=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_usage
                exit 0
                ;;
            -s|--status)
                show_status_only=true
                shift
                ;;
            -y|--yes)
                skip_confirmation=true
                shift
                ;;
            *)
                print_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
    
    # Check prerequisites
    check_root
    check_lvm_tools
    
    if [[ "$show_status_only" == true ]]; then
        show_status
        exit 0
    fi
    
    # Show current status
    show_status
    
    # Perform cleanup
    cleanup_lvm
    
    # Show final status
    echo
    print_status "Final LVM status:"
    show_status
}

# Run main function
main "$@"
