{pkgs}:
pkgs.mkShell {
  buildInputs = with pkgs; [
    go_1_27
    gopls
    go-tools
    gotools
    git
    gh
  ];

  shellHook = ''
    export GOFLAGS="-mod=readonly"
    echo "oci-sync dev shell"
    echo "Run 'go test ./...' to run tests"
    echo "Run 'go build ./...' to build"
  '';
}
