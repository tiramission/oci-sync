{
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.programs.oci-sync;
  cfgFile = lib.generators.toYAML {} cfg.settings;
in {
  meta.maintainers = [lib.maintainers.tiramission];

  options.programs.oci-sync = {
    enable = lib.mkEnableOption "oci-sync";

    package = lib.mkOption {
      type = lib.types.package;
      defaultText = "pkgs.oci-sync";
      description = "oci-sync package to install";
    };

    settings = lib.mkOption {
      type = lib.types.submodule {
        options = {
          shortcuts = lib.mkOption {
            type = lib.types.attrsOf (lib.types.submodule {
              options = {
                repo = lib.mkOption {
                  type = lib.types.str;
                  default = "";
                  description = "Default repository for this shortcut command";
                };
              };
            });
            default = {};
            description = "Shortcut commands mapped to their default repositories";
          };
          auths = lib.mkOption {
            type = lib.types.attrsOf (lib.types.submodule {
              options = {
                username = lib.mkOption {
                  type = lib.types.str;
                  default = "";
                  description = "Registry authentication username";
                };
                password = lib.mkOption {
                  type = lib.types.str;
                  default = "";
                  description = "Registry authentication password or token";
                };
              };
            });
            default = {};
            description = "Per-registry authentication credentials";
          };
        };
      };
      default = {};
      description = "oci-sync configuration";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [cfg.package];

    xdg.configFile."oci-sync/oci-sync.yaml".text = cfgFile;
  };
}