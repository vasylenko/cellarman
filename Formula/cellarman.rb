class Cellarman < Formula
  desc "Terminal UI for Homebrew: browse, search, upgrade, and run diagnostics"
  homepage "https://github.com/vasylenko/cellarman"
  url "https://github.com/vasylenko/cellarman/archive/refs/tags/v0.2.2.tar.gz"
  sha256 "bcbba2f16ebfd9eb1a806217c84b966ce0896b3f49bd58b9995b8e22eceabeca"
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
