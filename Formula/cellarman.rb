class Cellarman < Formula
  desc "Terminal UI for Homebrew: browse, search, upgrade, and run diagnostics"
  homepage "https://github.com/vasylenko/cellarman"
  url "https://github.com/vasylenko/cellarman/archive/refs/tags/v0.2.1.tar.gz"
  sha256 "bc3844e8d0797cbd19d6182d5fad0cfd608d9189c870a63f7f003ba1f7245ffc"
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
