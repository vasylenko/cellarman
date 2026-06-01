class Cellarman < Formula
  desc "Terminal UI for Homebrew: browse, search, upgrade, and run diagnostics"
  homepage "https://github.com/vasylenko/cellarman"
  url "https://github.com/vasylenko/cellarman/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "4a8867459ae6d6f087eb8887c11d608f5941d5fec816b7bcb4c3f5ad1709cc90"
  license "MIT"
  head "https://github.com/vasylenko/cellarman.git", branch: "main"

  depends_on "go" => :build

  def install
    # -X stamps the release tag into the binary so `cellarman --version` and the
    # formula's version stay in lock-step; -s -w drop debug info to shrink it.
    ldflags = %W[-s -w -X main.version=#{version}]
    system "go", "build", *std_go_args(ldflags:), "./cmd/cellarman"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/cellarman --version")
  end
end
