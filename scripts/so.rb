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
    log = buildpath/"web-build.log"
    cd "web" do
      cmd = "npm install --ignore-scripts >>#{log} 2>&1 && npm run build >>#{log} 2>&1"
      unless quiet_system("sh", "-c", cmd)
        odie "UI build failed. See #{log}"
      end
      standalone = buildpath/"web/.next/standalone"
      odie "web UI standalone build missing" unless (standalone/"server.js").exist?
      static_dir = buildpath/"web/.next/static"
      if static_dir.exist?
        (standalone/".next/static").mkpath
        static_dir.children.each { |child| cp_r child, standalone/".next/static"/child.basename }
      end
      public_dir = buildpath/"web/public"
      if public_dir.exist?
        (standalone/"public").mkpath
        public_dir.children.each { |child| cp_r child, standalone/"public"/child.basename }
      end
      dst = share/"superopen/web"
      dst.mkpath
      standalone.children.each { |child| cp_r child, dst/child.basename }
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
