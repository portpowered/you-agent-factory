cask "infinite-you" do
  arch arm: "arm64", intel: "amd64"

  version "0.0.2"
  sha256 arm:   "c5551530a288d4c2ea0cd52cfd30ef77dad18876339bc4f91a046a334437234a",
         intel: "144cbc7afeff74af11953589e89f492919a955a7a2c657fbb8df08b976002771"

  url "https://github.com/portpowered/infinite-you/releases/download/v#{version}/infinite-you_#{version}_darwin_#{arch}.tar.gz",
      verified: "github.com/portpowered/infinite-you/"
  name "Infinite You"
  desc "AI agent factory CLI for scheduling and orchestrating concurrent AI work"
  homepage "https://github.com/portpowered/infinite-you"

  binary "infinite-you"

  caveats <<~EOS
    `infinite-you` is currently distributed without Apple code signing or notarization.
    If macOS still blocks launch after install, run:
      xattr -dr com.apple.quarantine "$(brew --prefix)/bin/infinite-you"
  EOS
end
