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
        packages.default = pkgs.buildGoModule {
          pname = "oci-sync";
          version = "0.1.0";
          src = ./.;

          vendorHash = "sha256-zBWOEbsRqPKGmDlvtYenXcfzk6KpVpj2X1Y9nh8/nFE=";

          nativeBuildInputs = [pkgs.installShellFiles];

          postInstall = ''
            installShellCompletion --cmd oci-sync \
              --bash <($out/bin/oci-sync completion bash) \
              --zsh <($out/bin/oci-sync completion zsh) \
              --fish <($out/bin/oci-sync completion fish)
          '';

          meta = with pkgs.lib; {
            description = "Sync local files to OCI-compatible image registries";
            homepage = "https://github.com/tiramission/oci-sync";
            license = licenses.mit;
            maintainers = [];
            mainProgram = "oci-sync";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            go-tools
          ];
        };
      }
    );
}
