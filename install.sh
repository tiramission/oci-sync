#!/usr/bin/env bash
set -euo pipefail

REPO="tiramission/oci-sync"
INSTALL_DIR="${HOME}/.local/bin"
BINARY_NAME="oci-sync"

main() {
    echo "Installing oci-sync from GitHub releases..."

    detect_os
    detect_arch
    fetch_latest_version
    download_binary
    install_binary
    cleanup

    echo ""
    echo "✅ Installation complete!"
    echo "   Binary installed to: ${INSTALL_DIR}/${BINARY_NAME}"
    echo ""
    echo "Add ${INSTALL_DIR} to your PATH if not already included:"
    echo "   export PATH=\"\${HOME}/.local/bin:\${PATH}\""
}

detect_os() {
    case "$(uname -s)" in
        Linux*)     OS="linux" ;;
        Darwin*)    OS="darwin" ;;
        *)          echo "❌ Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
    esac
    echo "Detected OS: ${OS}"
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)     ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        armv7l)     ARCH="arm" ;;
        *)          echo "❌ Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac
    echo "Detected architecture: ${ARCH}"
}

fetch_latest_version() {
    VERSION=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "${VERSION}" ]; then
        echo "❌ Failed to fetch latest release version" >&2
        exit 1
    fi

    echo "Latest version: ${VERSION}"
}

download_binary() {
    FILENAME="${BINARY_NAME}-${OS}-${ARCH}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

    echo "Downloading from: ${DOWNLOAD_URL}"

    curl -sSL -o "${FILENAME}" "${DOWNLOAD_URL}"
}

install_binary() {
    mkdir -p "${INSTALL_DIR}"

    mv "${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
}

cleanup() {
    rm -f "${FILENAME}" 2>/dev/null || true
}

main "$@"
