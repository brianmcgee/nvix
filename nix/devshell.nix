{
  inputs,
  lib,
  ...
}: {
  imports = [
    inputs.devshell.flakeModule
    inputs.process-compose-flake.flakeModule
  ];

  config.perSystem = {
    pkgs,
    config,
    ...
  }: let
    inherit (pkgs.stdenv) isLinux isDarwin;
  in {
    config.devshells.default = {
      env = [
        {
          name = "GOROOT";
          value = pkgs.go + "/share/go";
        }
        {
          name = "LD_LIBRARY_PATH";
          value = "$DEVSHELL_DIR/lib";
        }
      ];

      packages = with lib;
        mkMerge [
          [
            # golang
            pkgs.go
            pkgs.gotools
            pkgs.delve
            pkgs.golangci-lint

            pkgs.openssl

            pkgs.qemu-utils

            pkgs.statix
          ]
          # platform dependent CGO dependencies
          (mkIf isLinux [
            pkgs.gcc
          ])
          (mkIf isDarwin [
            pkgs.darwin.cctools
          ])
        ];

      commands = [
        {
          category = "development";
          package = pkgs.enumer;
        }
        {
          category = "development";
          package = pkgs.evans;
        }
      ];
    };
  };
}
