{pkgs}:
pkgs.buildGoModule {
  pname = "oci-sync";
  version = "0.6.0";
  src = ../.;

  vendorHash = "sha256-UW8V4lM4fpEFlvRWsiGWuADHLSIhbmThugdYdAV28S0=";

  nativeBuildInputs = [pkgs.installShellFiles];
  env.CGO_ENABLED = 0;

  postInstall = ''
    installShellCompletion --cmd oci-sync \
      --bash <($out/bin/oci-sync completion bash) \
      --zsh <($out/bin/oci-sync completion zsh) \
      --fish <($out/bin/oci-sync completion fish)
  '';

  meta = {
    description = "Sync local files to OCI-compatible image registries";
    homepage = "https://github.com/tiramission/oci-sync";
    license = pkgs.lib.licenses.mit;
    maintainers = with pkgs.lib.maintainers; [tiramission];
    mainProgram = "oci-sync";
    platforms = pkgs.lib.platforms.all;
  };
}
