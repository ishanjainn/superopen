# typed: strict
# frozen_string_literal: true

# Homebrew formula for the Superopen CLI (`so`).
#
# Development-only formula. Released binaries live in the published tap:
#   brew install ishanjainn/superopen/so
# To build the current checkout instead:
#   brew install --HEAD ./scripts/so.rb
class So < Formula
  desc "Superopen - harness engineering for AI coding agents"
  homepage "https://github.com/ishanjainn/superopen"
  license "Apache-2.0"
  head "https://github.com/ishanjainn/superopen.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", "-o", bin/"so", "./cmd/so"
  end

  test do
    assert_match "so", shell_output("#{bin}/so --help")
  end
end
