{inputs, ...}: {
  imports = [
    inputs.flake-parts.flakeModules.easyOverlay
  ];

  perSystem = {
    lib,
    pkgs,
    self',
    inputs',
    ...
  }: {
    packages = rec {
      nvix = inputs'.gomod2nix.legacyPackages.buildGoApplication rec {
        pname = "nvix";
        version = "0.0.1+dev";

        # ensure we are using the same version of go to build with
        inherit (pkgs) go;

        src = ../.;
        modules = ../gomod2nix.toml;

        ldflags = [
          "-X 'build.Name=${pname}'"
          "-X 'build.Version=${version}'"
        ];

        meta = with lib; {
          description = "NVIX: a NATS-based store for TVIX";
          homepage = "https://github.com/brianmcgee/nvix";
          license = licenses.mit;
          mainProgram = "nvix";
        };
      };

      default = nvix;
    };

    overlayAttrs = self'.packages;
  };
}
