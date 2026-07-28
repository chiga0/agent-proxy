#!/usr/bin/env bash
# agent-proxy installer
# Usage: curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash
#    or: curl -fsSL ... | bash -s -- --version v0.3.0
#    or: curl -fsSL ... | bash -s -- --mirror oss   (China mirror)
set -euo pipefail

REPO="chiga0/agent-proxy"
BINARY="agent-proxy"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION=""
MIRROR="${MIRROR:-auto}"  # auto | github | oss
OSS_BASE="https://agent-proxy.oss-cn-hangzhou.aliyuncs.com"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}==>${NC} $*"; }
warn()  { echo -e "${YELLOW}warning:${NC} $*"; }
error() { echo -e "${RED}error:${NC} $*" >&2; exit 1; }

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --dir)     INSTALL_DIR="$2"; shift 2 ;;
    --mirror)  MIRROR="$2"; shift 2 ;;
    *)         shift ;;
  esac
done

# Detect OS
detect_os() {
  case "$(uname -s)" in
    Darwin*) echo "darwin" ;;
    Linux*)  echo "linux" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) error "Unsupported OS: $(uname -s)" ;;
  esac
}

# Detect arch
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) error "Unsupported arch: $(uname -m)" ;;
  esac
}

# Get latest version
get_latest_version() {
  # Try OSS manifest first if mirror is oss
  if [[ "$MIRROR" == "oss" ]]; then
    local ver
    ver=$(curl -fsSL "${OSS_BASE}/releases/latest/manifest.json" 2>/dev/null \
      | grep '"version"' | head -1 | sed 's/.*"version": *"//;s/".*//' || true)
    [[ -n "$ver" ]] && echo "$ver" && return
  fi

  # Try GitHub
  if command -v gh &>/dev/null; then
    gh release view --repo "$REPO" --json tagName -q '.tagName' 2>/dev/null && return
  fi
  if command -v curl &>/dev/null; then
    local ver
    ver=$(curl -fsSL --connect-timeout 5 "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
      | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//' || true)
    [[ -n "$ver" ]] && echo "$ver" && return
  fi

  # Fallback to OSS
  if command -v curl &>/dev/null; then
    local ver
    ver=$(curl -fsSL "${OSS_BASE}/releases/latest/manifest.json" 2>/dev/null \
      | grep '"version"' | head -1 | sed 's/.*"version": *"//;s/".*//' || true)
    [[ -n "$ver" ]] && echo "$ver" && return
  fi

  error "Cannot determine latest version"
}

# Download file
download() {
  local url="$1" dest="$2"
  if command -v curl &>/dev/null; then
    curl -fsSL --retry 3 -o "$dest" "$url"
  elif command -v wget &>/dev/null; then
    wget -q --tries=3 -O "$dest" "$url"
  else
    error "Need curl or wget to download"
  fi
}

main() {
  local os arch version tag archive_url tmp_dir

  os=$(detect_os)
  arch=$(detect_arch)

  info "Detected: ${os}/${arch}"

  # Determine version
  if [[ -z "$VERSION" ]]; then
    info "Fetching latest version..."
    tag=$(get_latest_version)
    [[ -z "$tag" ]] && error "No releases found"
    VERSION="${tag#v}"
  else
    VERSION="${VERSION#v}"
    tag="v${VERSION}"
  fi
  info "Version: ${tag}"

  # Resolve mirror
  if [[ "$MIRROR" == "auto" ]]; then
    if curl -fsSL --connect-timeout 3 -o /dev/null "https://github.com" 2>/dev/null; then
      MIRROR="github"
    else
      info "GitHub unreachable, using OSS mirror"
      MIRROR="oss"
    fi
  fi

  # Build download URL
  local ext="tar.gz"
  [[ "$os" == "windows" ]] && ext="zip"
  local filename="${BINARY}_${VERSION}_${os}_${arch}.${ext}"

  if [[ "$MIRROR" == "oss" ]]; then
    archive_url="${OSS_BASE}/releases/${tag}/${filename}"
    info "Mirror: OSS (cn-hangzhou)"
  else
    archive_url="https://github.com/${REPO}/releases/download/${tag}/${filename}"
    info "Mirror: GitHub"
  fi

  # Download
  tmp_dir=$(mktemp -d)
  trap 'rm -rf "$tmp_dir"' EXIT

  info "Downloading ${archive_url}..."
  download "$archive_url" "${tmp_dir}/archive.${ext}"

  # Verify checksum
  local checksums_url
  if [[ "$MIRROR" == "oss" ]]; then
    checksums_url="${OSS_BASE}/releases/${tag}/checksums.txt"
  else
    checksums_url="https://github.com/${REPO}/releases/download/${tag}/checksums.txt"
  fi
  info "Verifying checksum..."
  if download "$checksums_url" "${tmp_dir}/checksums.txt" 2>/dev/null; then
    local expected_sha
    expected_sha=$(grep "$filename" "${tmp_dir}/checksums.txt" | awk '{print $1}')
    if [[ -n "$expected_sha" ]]; then
      local actual_sha
      if command -v sha256sum &>/dev/null; then
        actual_sha=$(sha256sum "${tmp_dir}/archive.${ext}" | awk '{print $1}')
      elif command -v shasum &>/dev/null; then
        actual_sha=$(shasum -a 256 "${tmp_dir}/archive.${ext}" | awk '{print $1}')
      else
        warn "No sha256sum or shasum found — skipping checksum verification"
        actual_sha="$expected_sha"
      fi
      if [[ "$actual_sha" != "$expected_sha" ]]; then
        error "Checksum mismatch! Expected: $expected_sha Got: $actual_sha"
      fi
      info "Checksum verified ✓"
    else
      warn "No checksum entry for $filename — skipping verification"
    fi
  else
    warn "Could not download checksums.txt — skipping verification"
  fi

  # Extract
  info "Extracting..."
  if [[ "$ext" == "tar.gz" ]]; then
    tar xzf "${tmp_dir}/archive.${ext}" -C "$tmp_dir"
  else
    unzip -qo "${tmp_dir}/archive.${ext}" -d "$tmp_dir"
  fi

  # Find binary
  local bin_path
  bin_path=$(find "$tmp_dir" -name "${BINARY}" -o -name "${BINARY}.exe" | head -1)
  [[ -z "$bin_path" ]] && error "Binary not found in archive"

  # Install
  if [[ ! -d "$INSTALL_DIR" ]]; then
    info "Creating ${INSTALL_DIR}..."
    mkdir -p "$INSTALL_DIR" 2>/dev/null || sudo mkdir -p "$INSTALL_DIR"
  fi

  if [[ -w "$INSTALL_DIR" ]]; then
    cp "$bin_path" "${INSTALL_DIR}/${BINARY}"
    chmod +x "${INSTALL_DIR}/${BINARY}"
  else
    info "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo cp "$bin_path" "${INSTALL_DIR}/${BINARY}"
    sudo chmod +x "${INSTALL_DIR}/${BINARY}"
  fi

  # macOS: remove quarantine attribute to avoid Gatekeeper block
  if [[ "$os" == "darwin" ]]; then
    xattr -d com.apple.quarantine "${INSTALL_DIR}/${BINARY}" 2>/dev/null || \
    sudo xattr -d com.apple.quarantine "${INSTALL_DIR}/${BINARY}" 2>/dev/null || true
  fi

  echo ""
  info "Installed ${BINARY} ${tag} to ${INSTALL_DIR}/${BINARY}"
  echo ""
  echo "  Quick start:"
  echo "    ${BINARY} init        # Interactive setup"
  echo "    ${BINARY} on          # Enable proxy"
  echo "    ${BINARY} doctor      # Verify"
  echo ""
}

main
