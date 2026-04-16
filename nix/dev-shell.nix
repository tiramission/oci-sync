{ pkgs }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    gopls
    go-tools
  ];
}
