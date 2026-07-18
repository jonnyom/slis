#!/usr/bin/env bash
# Cross-compile the JS TUI (`slis-ui`) for every release platform.
#
# `bun build --compile` embeds one platform's native libraries (OpenTUI's Zig
# core + ghostty's VT addon), so each target needs its own build. Bun's compile
# is platform-aware and only needs the *target* platform's optional npm packages
# present, so `bun install --cpu '*' --os '*'` pulls every `@opentui/core-<plat>`
# up front and one host can produce all four binaries.
#
# Output: tui-js/dist/<goos>-<goarch>/slis-ui — laid out with GOOS/GOARCH names
# (not Bun's) so .goreleaser.yaml can pick each one with {{ .Os }}/{{ .Arch }}.
# Run from anywhere; GoReleaser calls it as a `before` hook from the repo root.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tui_dir="${script_dir}/../tui-js"

cd "${tui_dir}"

echo "==> installing tui-js deps for all platforms"
bun install --frozen-lockfile --cpu '*' --os '*'

# GOOS/GOARCH  ->  bun --target triple
targets=(
  "darwin/amd64:bun-darwin-x64"
  "darwin/arm64:bun-darwin-arm64"
  "linux/amd64:bun-linux-x64"
  "linux/arm64:bun-linux-arm64"
)

for entry in "${targets[@]}"; do
  platform="${entry%%:*}"
  bun_target="${entry##*:}"
  goos="${platform%%/*}"
  goarch="${platform##*/}"
  out_dir="dist/${goos}-${goarch}"
  mkdir -p "${out_dir}"
  echo "==> compiling slis-ui for ${goos}/${goarch} (${bun_target})"
  bun build --compile --target="${bun_target}" ./src/index.tsx --outfile "${out_dir}/slis-ui"
  chmod +x "${out_dir}/slis-ui"
done

echo "==> slis-ui built for all platforms:"
ls -la dist/*/slis-ui
