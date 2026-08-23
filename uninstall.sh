#!/usr/bin/env bash
# uninstall.sh — completely remove lazyaddons from the system.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/pentsec/lazyaddons/main/uninstall.sh | bash
#
# Or download and run:
#   curl -sLO https://raw.githubusercontent.com/pentsec/lazyaddons/main/uninstall.sh
#   chmod +x uninstall.sh
#   ./uninstall.sh
#
# Options:
#   --force           Skip confirmation prompt
#   --keep-config     Preserve configuration directory
#   --keep-data       Preserve WoW addon data (.lazyaddons, .backup)
#   -h, --help        Show this help

set -euo pipefail

BINARY_NAME="lazyaddons"

# ============================================================================
# Color support
# ============================================================================

setup_colors() {
    if [ -t 1 ] && [ "${TERM:-}" != "dumb" ]; then
        RED='\033[0;31m'
        GREEN='\033[0;32m'
        YELLOW='\033[1;33m'
        BLUE='\033[0;34m'
        CYAN='\033[0;36m'
        BOLD='\033[1m'
        DIM='\033[2m'
        NC='\033[0m'
    else
        RED='' GREEN='' YELLOW='' BLUE='' CYAN='' BOLD='' DIM='' NC=''
    fi
}

# ============================================================================
# Logging helpers
# ============================================================================

info()    { echo -e "${BLUE}[info]${NC}    $*"; }
success() { echo -e "${GREEN}[ok]${NC}      $*"; }
warn()    { echo -e "${YELLOW}[warn]${NC}    $*"; }
error()   { echo -e "${RED}[error]${NC}   $*" >&2; }
fatal()   { error "$@"; exit 1; }
step()    { echo -e "\n${CYAN}${BOLD}==>${NC} ${BOLD}$*${NC}"; }

# ============================================================================
# Help
# ============================================================================

show_help() {
    cat <<EOF
${BOLD}lazyaddons uninstaller${NC}

Usage: uninstall.sh [OPTIONS]

Options:
  --force           Skip confirmation prompt
  --keep-config     Preserve configuration directory
  --keep-data       Preserve WoW addon data (.lazyaddons, .backup)
  -h, --help        Show this help

Examples:
  ./uninstall.sh
  ./uninstall.sh --force
  ./uninstall.sh --keep-config --keep-data

EOF
}

# ============================================================================
# Detect installed components
# ============================================================================

BINARY_PATHS=()
CONFIG_DIR=""
CONFIG_FILE=""
WOW_PATHS=()

detect_binary() {
    step "Detecting binary"

    # Check PATH first
    if command -v "$BINARY_NAME" &>/dev/null; then
        local resolved
        resolved="$(command -v "$BINARY_NAME")"
        BINARY_PATHS+=("$resolved")
        success "Found in PATH: ${resolved}"
        return
    fi

    # Check common install locations
    local candidates=(
        "/usr/local/bin/${BINARY_NAME}"
        "${HOME}/.local/bin/${BINARY_NAME}"
    )

    for path in "${candidates[@]}"; do
        if [ -x "$path" ] || [ -f "$path" ]; then
            BINARY_PATHS+=("$path")
            success "Found: ${path}"
            return
        fi
    done

    warn "Binary not found — may already be removed"
}

detect_config() {
    step "Detecting configuration"

    # XDG_CONFIG_HOME or ~/.config
    local config_base="${XDG_CONFIG_HOME:-${HOME}/.config}"
    local candidate="${config_base}/${BINARY_NAME}"

    if [ -d "$candidate" ]; then
        CONFIG_DIR="$candidate"
        CONFIG_FILE="${candidate}/config.json"
        success "Config directory: ${candidate}"
    else
        warn "Config directory not found — may already be removed"
    fi
}

detect_wow_data() {
    step "Detecting WoW addon data"

    if [ -z "$CONFIG_FILE" ] || [ ! -f "$CONFIG_FILE" ]; then
        warn "No config file found — cannot detect WoW paths"
        return
    fi

    # Extract wow_path values from config (supports v1 root-level and v2 profiles).
    # Uses grep/sed to avoid requiring jq.
    local paths
    paths="$(grep -o '"wow_path"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" 2>/dev/null \
        | sed 's/.*"wow_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' \
        || true)"

    if [ -z "$paths" ]; then
        warn "No WoW paths found in config"
        return
    fi

    local seen=()
    while IFS= read -r wp; do
        [ -z "$wp" ] && continue

        # Skip duplicates
        local skip=false
        for s in "${seen[@]+"${seen[@]}"}"; do
            [ "$s" = "$wp" ] && skip=true && break
        done
        $skip && continue
        seen+=("$wp")

        # Check for .lazyaddons and .backup directories
        local addons_dir="${wp}"
        local lazy_dir="${addons_dir}/.lazyaddons"
        local backup_dir="${addons_dir}/.backup"

        if [ -d "$lazy_dir" ] || [ -d "$backup_dir" ]; then
            WOW_PATHS+=("$addons_dir")
            success "WoW AddOns: ${addons_dir}"
        fi
    done <<< "$paths"

    if [ ${#WOW_PATHS[@]} -eq 0 ]; then
        warn "No .lazyaddons or .backup directories found in WoW paths"
    fi
}

# ============================================================================
# Display removal plan
# ============================================================================

show_removal_plan() {
    step "Removal plan"

    echo ""
    echo -e "${BOLD}The following will be removed:${NC}"
    echo ""

    local has_items=false

    if [ ${#BINARY_PATHS[@]} -gt 0 ]; then
        has_items=true
        echo -e "  ${CYAN}Binary:${NC}"
        for path in "${BINARY_PATHS[@]}"; do
            echo -e "    - ${path}"
        done
    fi

    if [ -n "$CONFIG_DIR" ] && [ "${KEEP_CONFIG:-false}" != "true" ]; then
        has_items=true
        echo -e "  ${CYAN}Configuration:${NC}"
        echo -e "    - ${CONFIG_DIR}"
    fi

    if [ ${#WOW_PATHS[@]} -gt 0 ] && [ "${KEEP_DATA:-false}" != "true" ]; then
        has_items=true
        echo -e "  ${CYAN}WoW addon data:${NC}"
        for wp in "${WOW_PATHS[@]}"; do
            echo -e "    - ${wp}/.lazyaddons/"
            echo -e "    - ${wp}/.backup/"
        done
    fi

    if [ "${KEEP_CONFIG:-false}" = "true" ]; then
        echo -e "  ${DIM}Configuration preserved (--keep-config)${NC}"
    fi

    if [ "${KEEP_DATA:-false}" = "true" ]; then
        echo -e "  ${DIM}WoW addon data preserved (--keep-data)${NC}"
    fi

    if [ "$has_items" = false ]; then
        echo -e "  ${DIM}(nothing to remove)${NC}"
        echo ""
        success "lazyaddons appears to be already uninstalled"
        exit 0
    fi

    echo ""
    echo -e "  ${YELLOW}Addon folders inside AddOns/ (Details, DragonUI, etc.) will NOT be removed.${NC}"
    echo -e "  ${DIM}lazyaddons never modifies addon content — it only manages git repos.${NC}"
    echo ""
}

# ============================================================================
# Confirm
# ============================================================================

confirm_removal() {
    if [ "${FORCE:-false}" = "true" ]; then
        return 0
    fi

    echo -en "${YELLOW}Proceed with uninstall? [y/N] ${NC}"
    read -r answer
    case "$answer" in
        [yY]|[yY][eE][sS])
            echo ""
            ;;
        *)
            echo ""
            info "Uninstall cancelled"
            exit 0
            ;;
    esac
}

# ============================================================================
# Remove binary
# ============================================================================

remove_binary() {
    step "Removing binary"

    if [ ${#BINARY_PATHS[@]} -eq 0 ]; then
        warn "No binary found — skipping"
        return
    fi

    for path in "${BINARY_PATHS[@]}"; do
        if [ -f "$path" ] || [ -x "$path" ]; then
            info "Removing ${path}..."
            if rm "$path" 2>/dev/null; then
                success "Removed ${path}"
            elif command -v sudo &>/dev/null; then
                warn "Permission denied. Trying with sudo..."
                sudo rm "$path"
                success "Removed ${path} (via sudo)"
            else
                error "Failed to remove ${path} — permission denied"
            fi
        fi
    done
}

# ============================================================================
# Remove configuration
# ============================================================================

remove_config() {
    step "Removing configuration"

    if [ "${KEEP_CONFIG:-false}" = "true" ]; then
        info "Skipping — --keep-config specified"
        return
    fi

    if [ -z "$CONFIG_DIR" ]; then
        warn "No config directory found — skipping"
        return
    fi

    if [ ! -d "$CONFIG_DIR" ]; then
        warn "Config directory does not exist — skipping"
        return
    fi

    info "Removing ${CONFIG_DIR}..."
    if rm -rf "$CONFIG_DIR"; then
        success "Removed ${CONFIG_DIR}"
    else
        error "Failed to remove ${CONFIG_DIR}"
    fi
}

# ============================================================================
# Remove WoW addon data
# ============================================================================

remove_wow_data() {
    step "Removing WoW addon data"

    if [ "${KEEP_DATA:-false}" = "true" ]; then
        info "Skipping — --keep-data specified"
        return
    fi

    if [ ${#WOW_PATHS[@]} -eq 0 ]; then
        warn "No WoW addon data found — skipping"
        return
    fi

    for wp in "${WOW_PATHS[@]}"; do
        local lazy_dir="${wp}/.lazyaddons"
        local backup_dir="${wp}/.backup"

        if [ -d "$lazy_dir" ]; then
            info "Removing ${lazy_dir}..."
            if rm -rf "$lazy_dir"; then
                success "Removed ${lazy_dir}"
            else
                error "Failed to remove ${lazy_dir}"
            fi
        fi

        if [ -d "$backup_dir" ]; then
            info "Removing ${backup_dir}..."
            if rm -rf "$backup_dir"; then
                success "Removed ${backup_dir}"
            else
                error "Failed to remove ${backup_dir}"
            fi
        fi
    done
}

# ============================================================================
# Clean PATH entries from shell profiles
# ============================================================================

clean_path_entries() {
    step "Checking PATH entries"

    local profiles=(
        "${HOME}/.bashrc"
        "${HOME}/.bash_profile"
        "${HOME}/.zshrc"
        "${HOME}/.profile"
        "${HOME}/.config/fish/config.fish"
    )

    local cleaned=false

    for profile in "${profiles[@]}"; do
        if [ ! -f "$profile" ]; then
            continue
        fi

        # Skip if already installed (we're removing, not adding)
        if ! grep -q "lazyaddons" "$profile" 2>/dev/null; then
            continue
        fi

        info "Found lazyaddons PATH entry in ${profile}"

        if [ "${FORCE:-false}" != "true" ]; then
            echo -en "${YELLOW}Remove PATH entry from ${profile}? [y/N] ${NC}"
            read -r answer
            case "$answer" in
                [yY]|[yY][eE][sS])
                    ;;
                *)
                    info "Skipping ${profile}"
                    continue
                    ;;
            esac
        fi

        # Create backup before modifying
        local backup="${profile}.lazyaddons-bak"
        cp "$profile" "$backup"

        # Remove lines that add lazyaddons to PATH
        local tmp="${profile}.tmp"
        grep -v "lazyaddons" "$profile" > "$tmp" || true
        mv "$tmp" "$profile"

        success "Cleaned ${profile} (backup: ${backup})"
        cleaned=true
    done

    if [ "$cleaned" = false ]; then
        info "No PATH entries to clean"
    fi
}

# ============================================================================
# Verify uninstall
# ============================================================================

verify_uninstall() {
    step "Verifying uninstall"

    local issues=0

    # Check binary
    if command -v "$BINARY_NAME" &>/dev/null; then
        error "Binary still in PATH: $(command -v "$BINARY_NAME")"
        issues=$((issues + 1))
    fi

    for path in "${BINARY_PATHS[@]}"; do
        if [ -f "$path" ] || [ -x "$path" ]; then
            error "Binary still exists: ${path}"
            issues=$((issues + 1))
        fi
    done

    # Check config
    if [ "${KEEP_CONFIG:-false}" != "true" ] && [ -n "$CONFIG_DIR" ] && [ -d "$CONFIG_DIR" ]; then
        error "Config directory still exists: ${CONFIG_DIR}"
        issues=$((issues + 1))
    fi

    # Check WoW data
    if [ "${KEEP_DATA:-false}" != "true" ]; then
        for wp in "${WOW_PATHS[@]}"; do
            if [ -d "${wp}/.lazyaddons" ]; then
                error ".lazyaddons still exists: ${wp}/.lazyaddons"
                issues=$((issues + 1))
            fi
            if [ -d "${wp}/.backup" ]; then
                error ".backup still exists: ${wp}/.backup"
                issues=$((issues + 1))
            fi
        done
    fi

    if [ "$issues" -eq 0 ]; then
        success "All components removed successfully"
    else
        warn "${issues} item(s) could not be removed — check permissions"
    fi
}

# ============================================================================
# Print summary
# ============================================================================

print_summary() {
    echo ""
    echo -e "${GREEN}${BOLD}Uninstall complete!${NC}"
    echo ""
    echo -e "${BOLD}Notes:${NC}"
    echo -e "  - Addon folders in AddOns/ were ${CYAN}not removed${NC} (managed independently)"
    echo -e "  - Restart your shell or run ${BOLD}hash -r${NC} to clear the cached binary path"
    if [ -n "$CONFIG_DIR" ] && [ "${KEEP_CONFIG:-false}" = "true" ]; then
        echo -e "  - Config preserved at: ${DIM}${CONFIG_DIR}${NC}"
    fi
    echo ""
    echo -e "${DIM}To reinstall: https://github.com/pentsec/lazyaddons${NC}"
    echo ""
}

# ============================================================================
# Main
# ============================================================================

main() {
    setup_colors

    # Parse arguments
    FORCE="false"
    KEEP_CONFIG="false"
    KEEP_DATA="false"

    while [ $# -gt 0 ]; do
        case "$1" in
            --force|-f)
                FORCE="true"; shift
                ;;
            --keep-config)
                KEEP_CONFIG="true"; shift
                ;;
            --keep-data)
                KEEP_DATA="true"; shift
                ;;
            -h|--help)
                setup_colors
                show_help
                exit 0
                ;;
            *)
                fatal "Unknown option: $1. Use --help for usage."
                ;;
        esac
    done

    echo ""
    echo -e "${CYAN}${BOLD}lazyaddons uninstaller${NC}"
    echo -e "${DIM}  Removing all lazyaddons components from your system${NC}"
    echo ""

    detect_binary
    detect_config
    detect_wow_data
    show_removal_plan
    confirm_removal

    remove_binary
    remove_config
    remove_wow_data
    clean_path_entries
    verify_uninstall
    print_summary
}

main "$@"
