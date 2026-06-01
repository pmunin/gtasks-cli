class Gtasks < Formula
  desc "CLI tool for Google Tasks"
  homepage "https://github.com/pmunin/gtasks-cli"
  url "ssh://git@github.com/pmunin/gtasks-cli.git", using: :git, tag: "${TAG}"
  license "Apache-2.0"

  head "ssh://git@github.com/pmunin/gtasks-cli.git", using: :git, branch: "master"

  depends_on "go" => :build

  def install
    system "go", "build", "-trimpath",
           "-ldflags", "-s -w -X github.com/pmunin/gtasks-cli/cmd.Version=${TAG}",
           "-o", bin/"gtasks", "."
  end

  def caveats
    <<~EOS
      gtasks ships without Google OAuth credentials — you must supply your own
      before `gtasks login` will work:

        1. In Google Cloud Console: enable the Google Tasks API and create an
           OAuth client (Application type: "Desktop app").
        2. Save the client ID/secret to ~/.config/gtasks/config.toml:

             [credentials]
             client_id     = "....apps.googleusercontent.com"
             client_secret = "...."

           then: chmod 600 ~/.config/gtasks/config.toml
           (or export GTASKS_CLIENT_ID / GTASKS_CLIENT_SECRET instead)
        3. Run: gtasks login
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gtasks --version")
  end
end
