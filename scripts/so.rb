# typed: strict
# frozen_string_literal: true

# Homebrew formula for the Superopen CLI (`so`).
#
# Development-only formula. Released binaries live in the published tap:
#   brew install ishanjainn/superopen/so
# To build the current checkout instead:
#   brew install --HEAD ./scripts/so.rb
class So < Formula
  desc "Native code graph and coding-session observability"
  homepage "https://github.com/ishanjainn/superopen"
  license "Apache-2.0"
  head "https://github.com/ishanjainn/superopen.git", branch: "main"

  depends_on "go" => :build
  depends_on "node"

  def install
    system "go", "build", "-o", bin/"so", "./cmd/so"
    dst = share/"superopen/web"
    dst.mkpath
    Dir.children("web").each do |name|
      next if name == "node_modules" || name == ".next"
      cp_r buildpath/"web"/name, dst/name
    end
    cd dst do
      system "npm", "install", "--ignore-scripts"
      system "npm", "run", "build"
    end
  end

  def caveats
    <<~EOS
      Run `so install` once to wire coding-agent hooks.
      Then in any repo: `so init` and `so dev`.
    EOS
  end

  test do
    assert_match "so", shell_output("#{bin}/so --help")
  end
end
