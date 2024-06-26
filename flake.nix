{
  description = "quickstart shenanigans";

  inputs =
    {
      nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.05";

      devshell.url = "github:numtide/devshell";
      flake-utils.url = "github:numtide/flake-utils";

      flake-compat = {
        url = "github:edolstra/flake-compat";
        flake = false;
      };
    };

  outputs = { self, flake-utils, devshell, nixpkgs, ... }:
    flake-utils.lib.eachDefaultSystem (system: {
      devShells.default =
        let
          pkgs = import nixpkgs {
            inherit system;

            overlays = [ devshell.overlays.default ];
          };
        in
        pkgs.devshell.mkShell {
          imports = [ (pkgs.devshell.importTOML ./devshell.toml) ];
          packages = with pkgs; [
            (pkgs.wrapHelm pkgs.kubernetes-helm { plugins = [ pkgs.kubernetes-helmPlugins.helm-diff ]; })
            pulumi-bin
            go_1_21
            kubectl
            jq
            libvirt
            helmfile-wrapped
            k9s
            cdrkit # for libvirt mkisofs
            gptfdisk
          ];
        };
    });
}
