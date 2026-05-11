{pkgs}:
pkgs.buildGoModule {
  pname = "oci-sync";
  version = "0.1.0-dev";
  src = ../.;

  vendorHash = "sha256-jD8v1jmMBZ4lGTDIx17L943tMWYZO6rv8Mrl13YfVEk=";

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
