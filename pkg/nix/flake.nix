# SPDX-FileCopyrightText: 2026 Sebastien Rousseau
# SPDX-License-Identifier: MIT OR Apache-2.0
#
# Template. `nix build` from this directory, or `nix run github:sebastienrousseau/draft`
# once this is moved to the repository root.
#
# vendorHash must be replaced: run the build once, and Nix prints the correct
# value in the mismatch error.
{
  description = "Turn research papers into grounded Markdown drafts";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule rec {
          pname = "draft";
          version = "0.0.32";

          src = ../..;

          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

          ldflags = [ "-s" "-w" "-X" "main.version=${version}" ];

          nativeBuildInputs = [ pkgs.installShellFiles ];

          # Generated from the CLI definitions, never committed.
          postInstall = ''
            $out/bin/draft --man > draft.1
            installManPage draft.1
            installShellCompletion --cmd draft \
              --bash <($out/bin/draft --completion bash) \
              --zsh  <($out/bin/draft --completion zsh) \
              --fish <($out/bin/draft --completion fish)
          '';

          # PDF support is optional at runtime; wrap only if you want it
          # guaranteed present.
          meta = with pkgs.lib; {
            description = "Turn research papers into grounded Markdown drafts";
            homepage = "https://draftlib.com";
            license = with licenses; [ mit asl20 ];
            mainProgram = "draft";
            platforms = platforms.unix;
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [ go_1_24 golangci-lint poppler_utils groff python3 nodejs ];
        };
      });
}
