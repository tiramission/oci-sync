{
  description = "oci-sync: Sync local files to OCI-compatible image registries";

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
        packages.default = pkgs.buildGoModule {
          pname = "oci-sync";
          version = "0.1.0";
          src = ./.;

          # Run `nix run nixpkgs#go -- mod vendor` if you prefer vendorHash = null
          vendorHash = "sha256-R4mwvb2nGNrP3o32QYtLEKdSG9b9gD80UHkHaJA0/2E=";

          # Avoid running network-dependent tests during packaging
          # doCheck = false;

          meta = with pkgs.lib; {
            description = "Sync local files to OCI-compatible image registries";
            homepage = "https://github.com/tiramission/oci-sync";
            license = licenses.mit;
            maintainers = [ ];
            mainProgram = "oci-sync";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ go gopls go-tools ];
        };
      }
    );
}
