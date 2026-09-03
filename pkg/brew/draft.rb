# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# Template for a Homebrew tap. Replace version and the sha256 values with those
# from the release's checksums.txt.
#
# GoReleaser can generate and publish this automatically: add a `brews:` block
# to .goreleaser.yaml once a homebrew-tap repository exists.
class Draft < Formula
  desc "Turn research papers into grounded Markdown drafts"
  homepage "https://draftlib.com"
  version "0.0.32"
  license any_of: ["MIT", "Apache-2.0"]

  on_macos do
    on_arm do
      url "https://github.com/sebastienrousseau/draft/releases/download/v#{version}/draft_#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACE_ME"
    end
    on_intel do
      url "https://github.com/sebastienrousseau/draft/releases/download/v#{version}/draft_#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACE_ME"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/sebastienrousseau/draft/releases/download/v#{version}/draft_#{version}_linux_arm64.tar.gz"
      sha256 "REPLACE_ME"
    end
    on_intel do
      url "https://github.com/sebastienrousseau/draft/releases/download/v#{version}/draft_#{version}_linux_amd64.tar.gz"
      sha256 "REPLACE_ME"
    end
  end

  # Recommended, not required: draft reports a missing tool with an actionable
  # message, and Markdown sources need nothing at all.
  depends_on "poppler" => :recommended

  def install
    bin.install "draft"
    man1.install "share/man/man1/draft.1"
    bash_completion.install "share/bash-completion/completions/draft"
    zsh_completion.install "share/zsh/site-functions/_draft"
    fish_completion.install "share/fish/vendor_completions.d/draft.fish"
    doc.install "README.md", "CHANGELOG.md", "LICENSE-MIT", "LICENSE-APACHE"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/draft --version")
    # --doctor exits non-zero when a requirement is missing, which is a valid
    # outcome in a sandbox; assert it produced its report instead.
    assert_match "BACKENDS", shell_output("#{bin}/draft --doctor", 1)
  end
end
