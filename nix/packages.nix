{inputs, ...}: {
  imports = [
    inputs.flake-parts.flakeModules.easyOverlay
  ];

  perSystem = {
    lib,
    pkgs,
    self',
    ...
  }: {
    packages = rec {
      nvix = pkgs.buildGoModule rec {
        pname = "nvix";
        version = "0.0.1+dev";

        src = ../.;
        vendorSha256 = "sha256-BswbommN6CfUiWk/DyP0LPUm3HmOyXtfZsSAAVMx5NY=";

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
