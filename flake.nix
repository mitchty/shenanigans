{
  description = "quickstart shenanigans";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";

    flakelight = {
      url = "github:nix-community/flakelight";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    { flakelight, ... }@inputs:
    flakelight ./. {
      #      imports = [ (inputs.nixpkgs.legacyPackages.x86_64-linux.pkgs.devshell.importTOML ./devshell.toml) ];
      devShells.default = {
        packages = with inputs.nixpkgs.legacyPackages.x86_64-linux.pkgs; [
          go
          libxslt
          (pkgs.wrapHelm pkgs.kubernetes-helm { plugins = [ pkgs.kubernetes-helmPlugins.helm-diff ]; })
          pulumi-bin
          pulumiPackages.pulumi-go
          go_1_24
          kubectl
          k8sgpt
          jq
          libvirt
          helmfile-wrapped
          k9s
          cdrkit # for libvirt mkisofs
          gptfdisk
          skopeo
          kubernetes-helm
        ];
      };
    };
}
