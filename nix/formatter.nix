{inputs, ...}: {
  imports = [
    inputs.treefmt-nix.flakeModule
  ];
  perSystem = {
    config,
    pkgs,
    ...
  }: {
    treefmt.config = {
      inherit (config.flake-root) projectRootFile;
      package = pkgs.treefmt;

      programs = {
        alejandra.enable = true;
        deadnix.enable = true;
        gofumpt.enable = true;
        prettier.enable = true;
        statix.enable = true;
      };

      settings.formatter.prettier.options = ["--tab-width" "4"];
    };

    formatter = config.treefmt.build.wrapper;

    devshells.default = {
      commands = [
        {
          category = "checks";
          name = "fmt";
          help = "Format the repo";
          command = "nix fmt";
        }
      ];
    };
  };
}
