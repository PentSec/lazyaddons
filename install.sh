#!/usr/bin/env bash
# install.sh — download the latest lazyaddons binary from GitHub Releases.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/pentsec/lazzyaddons/main/install.sh | bash
#
# Or download and run:
#   curl -sLO https://raw.githubusercontent.com/pentsec/lazzyaddons/main/install.sh
#   chmod +x install.sh
#   ./install.sh
#
# Options:
#   --dir DIR         Custom install directory (default: auto-detect)
#   --version TAG     Install a specific release tag (default: latest)
#   --insecure        Skip checksum verification (not recommended)
#   -h, --help        Show this help

set -euo pipefail

GITHUB_OWNER="pentsec"
GITHUB_REPO="lazyaddons"
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
${BOLD}lazyaddons installer${NC}

Usage: install.sh [OPTIONS]

Options:
  --dir DIR         Custom install directory (default: auto-detect)
  --version TAG     Install a specific release tag (default: latest)
  --insecure        Skip checksum verification (not recommended)
  -h, --help        Show this help

Examples:
  curl -sSL https://raw.githubusercontent.com/${GITHUB_OWNER}/${GITHUB_REPO}/main/install.sh | bash
  ./install.sh
  ./install.sh --dir \$HOME/.local/bin
  ./install.sh --version v0.1.0

EOF
}

# ============================================================================
# Platform detection
# ============================================================================

detect_platform() {
    local uname_os uname_arch

    uname_os="$(uname -s)"
    uname_arch="$(uname -m)"

    case "$uname_os" in
        Darwin) OS="darwin"; OS_LABEL="macOS" ;;
        Linux)  OS="linux";  OS_LABEL="Linux" ;;
        *)      fatal "Unsupported OS: $uname_os. Only macOS and Linux are supported." ;;
    esac

    case "$uname_arch" in
        x86_64|amd64)   ARCH="amd64" ;;
        arm64|aarch64)  ARCH="arm64" ;;
        *)              fatal "Unsupported architecture: $uname_arch. Only amd64 and arm64 are supported." ;;
    esac

    success "Platform: ${OS_LABEL} (${OS}/${ARCH})"
}

# ============================================================================
# Prerequisites
# ============================================================================

check_prerequisites() {
    step "Checking prerequisites"

    local missing=()

    if ! command -v curl &>/dev/null; then
        missing+=("curl")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        fatal "Missing required tools: ${missing[*]}. Please install them and try again."
    fi

    success "curl is available"
}

# ============================================================================
# Install directory
# ============================================================================

resolve_install_dir() {
    step "Determining install directory"

    if [ -n "${INSTALL_DIR:-}" ]; then
        success "Using custom directory: ${INSTALL_DIR}"
        return
    fi

    # Mirror the reference behaviour: prefer /usr/local/bin if writable,
    # fall back to ~/.local/bin for unprivileged installs.
    if [ -d "/usr/local/bin" ] && [ -w "/usr/local/bin" ]; then
        INSTALL_DIR="/usr/local/bin"
    elif [ "$(id -u)" = "0" ]; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="${HOME}/.local/bin"
    fi

    success "Install directory: ${INSTALL_DIR}"
}

# ============================================================================
# Install
# ============================================================================

install_binary() {
    step "Installing ${BINARY_NAME}"

    # Resolve version.
    local version=""
    if [ -z "${VERSION:-}" ]; then
        info "Fetching latest release from GitHub..."
        local response
        response="$(curl -sL -w "\n%{http_code}" "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/latest")" \
            || fatal "Failed to fetch latest release"

        local http_code body
        http_code="$(echo "$response" | tail -n1)"
        body="$(echo "$response" | sed '$d')"

        if [ "$http_code" != "200" ]; then
            fatal "GitHub API returned HTTP $http_code. Rate limited? Try again later."
        fi

        version="$(echo "$body" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"

        if [ -z "$version" ]; then
            fatal "Could not determine latest version from GitHub API response"
        fi

        success "Latest version: ${version}"
    else
        version="${VERSION}"
    fi

    local version_number="${version#v}"
    local archive_name="${BINARY_NAME}_${version_number}_${OS}_${ARCH}.tar.gz"
    local download_url="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${version}/${archive_name}"
    local checksums_url="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${version}/checksums.txt"

    # Create temp directory — clean up on exit
    local tmpdir
    tmpdir="$(mktemp -d)"
    trap '[ -n "${tmpdir:-}" ] && rm -rf "$tmpdir"' EXIT

    # Download archive
    info "Downloading ${archive_name}..."
    if ! curl -sfL -o "${tmpdir}/${archive_name}" "$download_url"; then
        fatal "Failed to download ${download_url}"
    fi

    # Verify file was actually downloaded (not a 404 HTML page)
    local file_size
    file_size="$(wc -c < "${tmpdir}/${archive_name}" | tr -d '[:space:]')"
    if [ "$file_size" -lt 1000 ]; then
        fatal "Downloaded file is suspiciously small (${file_size} bytes). Archive may not exist for this platform."
    fi

    success "Downloaded ${archive_name} (${file_size} bytes)"

    # Download and verify checksum — fail closed unless --insecure is set
    if [ "${INSECURE:-false}" != "true" ]; then
        info "Verifying checksum..."
        if curl -sL -o "${tmpdir}/checksums.txt" "$checksums_url"; then
            local expected_checksum
            expected_checksum="$(grep "${archive_name}" "${tmpdir}/checksums.txt" 2>/dev/null | awk '{print $1}' || true)"

            if [ -n "$expected_checksum" ]; then
                local actual_checksum
                if command -v sha256sum &>/dev/null; then
                    actual_checksum="$(sha256sum "${tmpdir}/${archive_name}" | awk '{print $1}')"
                elif command -v shasum &>/dev/null; then
                    actual_checksum="$(shasum -a 256 "${tmpdir}/${archive_name}" | awk '{print $1}')"
                else
                    fatal "No sha256sum or shasum tool found. Cannot verify checksum.\nInstall coreutils or use --insecure to skip (not recommended)."
                fi

                if [ "$actual_checksum" != "$expected_checksum" ]; then
                    fatal "Checksum mismatch!\n  Expected: ${expected_checksum}\n  Got:      ${actual_checksum}"
                fi
                success "Checksum verified"
            else
                fatal "Archive '${archive_name}' not found in checksums.txt. Refusing to install unverified binary.\nUse --insecure to skip (not recommended)."
            fi
        else
            fatal "Could not download checksums.txt from:\n  ${checksums_url}\nRefusing to install without integrity verification.\nUse --insecure to skip (not recommended)."
        fi
    else
        warn "Checksum verification skipped (--insecure)"
    fi

    # Extract binary
    info "Extracting ${BINARY_NAME}..."
    if ! tar -xzf "${tmpdir}/${archive_name}" -C "$tmpdir"; then
        fatal "Failed to extract archive"
    fi

    if [ ! -f "${tmpdir}/${BINARY_NAME}" ]; then
        fatal "Binary '${BINARY_NAME}' not found in archive"
    fi

    # Create install dir if needed
    mkdir -p "$INSTALL_DIR"

    # Install binary
    info "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
    if cp "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null; then
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    elif command -v sudo &>/dev/null; then
        warn "Permission denied. Trying with sudo..."
        sudo cp "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        fatal "Cannot write to ${INSTALL_DIR}. Run with sudo or use --dir to specify a writable directory."
    fi

    success "Installed ${BINARY_NAME} ${version} to ${INSTALL_DIR}/${BINARY_NAME}"

    # Check if install dir is in PATH
    if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
        warn "${INSTALL_DIR} is not in your PATH"
        echo ""
        warn "Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
        echo -e "  ${DIM}export PATH=\"\$PATH:${INSTALL_DIR}\"${NC}"
        echo ""
    fi
}

# ============================================================================
# Verify installation
# ============================================================================

verify_installation() {
    step "Verifying installation"

    # Allow PATH changes to take effect
    hash -r 2>/dev/null || true

    if command -v "$BINARY_NAME" &>/dev/null; then
        local version_output
        version_output="$("$BINARY_NAME" --version 2>&1 || true)"
        success "${BINARY_NAME} is installed: ${version_output}"
        return 0
    fi

    # Check common locations even if not in PATH
    local locations=(
        "/usr/local/bin/${BINARY_NAME}"
        "${HOME}/.local/bin/${BINARY_NAME}"
    )

    for loc in "${locations[@]}"; do
        if [ -n "$loc" ] && [ -x "$loc" ]; then
            local version_output
            version_output="$("$loc" --version 2>&1 || true)"
            success "Found ${BINARY_NAME} at ${loc}: ${version_output}"
            warn "Binary location is not in your PATH. Add it to use '${BINARY_NAME}' directly."
            return 0
        fi
    done

    warn "Could not verify installation. You may need to restart your shell."
    return 0
}

# ============================================================================
# Print next steps
# ============================================================================

print_next_steps() {
    echo ""
    echo -e "${GREEN}${BOLD}Installation complete!${NC}"
    echo ""
    echo -e "${BOLD}Next steps:${NC}"
    echo -e "  ${CYAN}1.${NC} Run ${BOLD}${BINARY_NAME}${NC} to set up your WoW addon manager"
    echo -e "  ${CYAN}2.${NC} Follow the interactive prompts to configure your WoW path"
    echo ""
    echo -e "${DIM}For help: ${BINARY_NAME} --help${NC}"
    echo -e "${DIM}Docs:     https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}${NC}"
    echo ""
}

# ============================================================================
# Main
# ============================================================================

main() {
    setup_colors

    # Parse arguments
    INSTALL_DIR=""
    VERSION=""
    INSECURE="false"

    while [ $# -gt 0 ]; do
        case "$1" in
            --dir)
                [ $# -lt 2 ] && fatal "--dir requires an argument"
                INSTALL_DIR="$2"; shift 2
                ;;
            --version)
                [ $# -lt 2 ] && fatal "--version requires an argument"
                VERSION="$2"; shift 2
                ;;
            --insecure)
                INSECURE="true"; shift
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

    step "Detecting platform"
    detect_platform

    check_prerequisites
    resolve_install_dir
    install_binary
    verify_installation
    print_next_steps
}

main "$@"
