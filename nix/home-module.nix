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
          experimental = lib.mkOption {
            type = lib.types.submodule {
              options = {
                enabled = lib.mkOption {
                  type = lib.types.bool;
                  default = true;
                  description = "Enable experimental commands";
                };
                repo = lib.mkOption {
                  type = lib.types.str;
                  default = "";
                  description = "Default repository for experimental commands";
                };
              };
            };
            default = {};
            description = "Experimental settings";
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
