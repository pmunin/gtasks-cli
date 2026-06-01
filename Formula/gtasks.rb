class Gtasks < Formula
  desc "CLI tool for Google Tasks"
  homepage "https://github.com/pmunin/gtasks-cli"
  url "git@github.com:pmunin/gtasks-cli.git", tag: "v0.13.0-pmunin"
  license "Apache-2.0"

  head "git@github.com:pmunin/gtasks-cli.git", branch: "master"

  depends_on "go" => :build

  def install
    system "go", "build", "-trimpath",
           "-ldflags", "-s -w -X github.com/pmunin/gtasks-cli/cmd.Version=v0.13.0-pmunin",
           "-o", bin/"gtasks", "."
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gtasks --version")
  end
end