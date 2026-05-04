cask "infinite-you" do
  arch arm: "arm64", intel: "amd64"

  version "0.0.1"
  sha256 arm:   "c2f30016121ff7ca89c626147f55307faf5240fde29bd9a491897b6fa3cb8b80",
         intel: "72ffab85b576fd9710b035dbfed0db739f5bf80009cad5e75939039b06eaa636"

  url "https://github.com/portpowered/infinite-you/releases/download/v#{version}/agent-factory_#{version}_darwin_#{arch}.tar.gz",
      verified: "github.com/portpowered/infinite-you/"
  name "Infinite You"
  desc "AI agent factory CLI for scheduling and orchestrating concurrent AI work"
  homepage "https://github.com/portpowered/infinite-you"

  binary "agent-factory", target: "infinite-you"

  caveats <<~EOS
    `infinite-you` is currently distributed without Apple code signing or notarization.
    If macOS still blocks launch after install, run:
      xattr -dr com.apple.quarantine "$(brew --prefix)/bin/infinite-you"
  EOS
end
