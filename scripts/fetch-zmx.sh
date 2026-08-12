#!/bin/sh
set -eu

version=0.7.0
base_url="https://github.com/neurosnap/zmx/releases/download/v${version}"

fetch_zmx() {
  goos="$1"
  goarch="$2"
  platform="$3"
  asset_arch="$4"
  expected="$5"
  output="third_party/zmx/dist/${goos}-${goarch}"
  archive="third_party/zmx/dist/zmx-${version}-${platform}-${asset_arch}.tar.gz"
  mkdir -p "$output"
  curl -fsSL "${base_url}/zmx-${version}-${platform}-${asset_arch}.tar.gz" -o "$archive"
  actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
  if [ "$actual" != "$expected" ]; then
    echo "zmx checksum mismatch for ${platform}-${asset_arch}" >&2
    exit 1
  fi
  tar -xzf "$archive" -C "$output"
  chmod 0755 "$output/zmx"
}

fetch_zmx darwin arm64 macos aarch64 a63d6f3edd6d4b38240f8f81513e60e35a898ca520211112d7bc67f610f1f3eb
fetch_zmx darwin amd64 macos x86_64 66c57e7963c84881266f9f3acfdb36945c340c016a57061948517f3b303ca7d3
fetch_zmx linux arm64 linux aarch64 77599f66124694fae80bbb1d2fa0eafdb8c648b427a048cad90513ecf6136fc9
fetch_zmx linux amd64 linux x86_64 8b8783d7b120c9ffd0acf4aee37969054dc0dfef3c4f3a4728d2efd35f2e97a0
