{
  description = "quickstart shenanigans";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/release-24.11";

    flakelight = {
      url = "github:nix-community/flakelight";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    { flakelight, ... }@inputs:
    flakelight ./. rec {
      #      imports = [ (pkgs.devshell.importTOML ./devshell.toml) ];
    };
}
