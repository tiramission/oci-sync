{
  description = "oci-sync: Sync local files to OCI-compatible image registries";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
  }:
    {
      homeModules = {
        oci-sync = ./nix;
        default = self.homeModules.oci-sync;
      };
    }
    // flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.callPackage ./nix/package.nix { inherit pkgs; };
        devShells.default = pkgs.callPackage ./nix/dev-shell.nix { inherit pkgs; };
      }
    );
}
