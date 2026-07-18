#!/usr/bin/env bash
# Render the compatibility Homebrew formula from GoReleaser's checksums.
# Existing users installed Slis as a formula, so keeping this recipe current
# lets `brew upgrade slis` move them to the same slis + slis-ui archive as the
# cask without a manual uninstall/migration.
set -euo pipefail

tag="${1:?usage: render-homebrew-formula.sh <tag> <checksums> <output>}"
checksums="${2:?usage: render-homebrew-formula.sh <tag> <checksums> <output>}"
output="${3:?usage: render-homebrew-formula.sh <tag> <checksums> <output>}"
version="${tag#v}"

checksum() {
  local filename="$1"
  local value
  value="$(awk -v filename="${filename}" '$2 == filename { print $1 }' "${checksums}")"
  if [[ -z "${value}" ]]; then
    echo "missing checksum for ${filename}" >&2
    exit 1
  fi
  printf '%s' "${value}"
}

darwin_amd64="$(checksum "slis_${version}_darwin_amd64.tar.gz")"
darwin_arm64="$(checksum "slis_${version}_darwin_arm64.tar.gz")"
linux_amd64="$(checksum "slis_${version}_linux_amd64.tar.gz")"
linux_arm64="$(checksum "slis_${version}_linux_arm64.tar.gz")"

mkdir -p "$(dirname "${output}")"
cat >"${output}" <<FORMULA
class Slis < Formula
  desc "Multi-repo worktree cockpit: a TUI + CLI for working across many git repos at once"
  homepage "https://github.com/jonnyom/slis"
  version "${version}"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/jonnyom/slis/releases/download/v#{version}/slis_#{version}_darwin_arm64.tar.gz"
      sha256 "${darwin_arm64}"
    else
      url "https://github.com/jonnyom/slis/releases/download/v#{version}/slis_#{version}_darwin_amd64.tar.gz"
      sha256 "${darwin_amd64}"
    end
    depends_on "terminal-notifier"
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/jonnyom/slis/releases/download/v#{version}/slis_#{version}_linux_arm64.tar.gz"
      sha256 "${linux_arm64}"
    else
      url "https://github.com/jonnyom/slis/releases/download/v#{version}/slis_#{version}_linux_amd64.tar.gz"
      sha256 "${linux_amd64}"
    end
  end

  def install
    bin.install "slis"
    bin.install "slis-ui"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/slis --version")
  end
end
FORMULA
