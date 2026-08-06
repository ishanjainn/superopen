# Homebrew formula for the Superopen CLI (`so`).
#
# Prefer the published tap once releases exist:
#   brew install superopen/so/so
#
# Or install from this file / HEAD:
#   brew install --HEAD ./scripts/so.rb
#
# Curl installer (no Homebrew required):
#   curl -fsSL https://raw.githubusercontent.com/superopen/so/main/scripts/install.sh | sh
class So < Formula
  desc "Superopen - harness engineering for AI coding agents"
  homepage "https://github.com/superopen/so"
  license "Apache-2.0"
  version "0.1.0"

  on_macos do
    on_arm do
      url "https://github.com/superopen/so/releases/download/cli-#{version}/so-darwin-arm64.tar.gz"
      # sha256 updated by the CLI release workflow / tap bump
    end
    on_intel do
      url "https://github.com/superopen/so/releases/download/cli-#{version}/so-darwin-amd64.tar.gz"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/superopen/so/releases/download/cli-#{version}/so-linux-arm64.tar.gz"
    end
    on_intel do
      url "https://github.com/superopen/so/releases/download/cli-#{version}/so-linux-amd64.tar.gz"
    end
  end

  head do
    url "https://github.com/superopen/so.git", branch: "main"
    depends_on "go" => :build
  end

  def install
    if build.head?
      system "go", "build", "-o", bin/"so", "./cmd/so"
    else
      bin.install Dir["so*"].first => "so"
    end
  end

  test do
    assert_match "so", shell_output("#{bin}/so --help")
  end
end
