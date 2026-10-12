#!/usr/bin/env bash
set -euo pipefail

# Run from any directory. GO may name an executable or an absolute path.
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
go_command="${GO:-go}"
skip_tests=false
if [[ "${1:-}" == "--skip-tests" ]]; then
    skip_tests=true
    shift
fi
if [[ $# -ne 0 ]]; then
    printf 'Usage: bash scripts/build.sh [--skip-tests]\n' >&2
    exit 2
fi
if ! command -v "$go_command" >/dev/null 2>&1; then
    printf 'Go was not found. Install Go or set GO=/path/to/go.\n' >&2
    exit 1
fi

cd -- "$project_dir"
export CGO_ENABLED=1
target_os="$("$go_command" env GOOS)"
host_os="$("$go_command" env GOHOSTOS)"
target_arch="$("$go_command" env GOARCH)"
host_arch="$("$go_command" env GOHOSTARCH)"
if [[ "$target_os" != "$host_os" || "$target_arch" != "$host_arch" ]]; then
    printf 'Native builds only: clear GOOS/GOARCH overrides; cross-compilation needs a separate C toolchain.\n' >&2
    exit 1
fi
case "$target_os" in
    linux|darwin) ;;
    *) printf 'Use scripts/build.ps1 on Windows. This script supports Linux and macOS.\n' >&2; exit 1 ;;
esac

"$go_command" version
if [[ "$skip_tests" == false ]]; then
    "$go_command" test ./...
fi
mkdir -p Build
"$go_command" build -trimpath -ldflags '-s -w' -o Build/GoDCP ./src
printf 'Built %s/Build/GoDCP\n' "$project_dir"
if [[ "$target_os" == darwin ]]; then
    bundle_dir=Build/GoDCP.app/Contents
    mkdir -p "$bundle_dir/MacOS"
    cp Build/GoDCP "$bundle_dir/MacOS/GoDCP"
    cat > "$bundle_dir/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleExecutable</key>
    <string>GoDCP</string>
    <key>CFBundleIdentifier</key>
    <string>org.godcp.editor</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>GoDCP</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>0.1.0</string>
    <key>CFBundleVersion</key>
    <string>0.1.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST
    printf 'Built unsigned %s/Build/GoDCP.app\n' "$project_dir"
fi
