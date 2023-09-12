{
  perSystem = {pkgs, ...}: {
    config.devshells.default = {
      commands = let
        category = "docs";
      in [
        {
          inherit category;
          package = pkgs.vhs;
        }
        {
          inherit category;
          help = "Generate all gifs used in docs";
          package = pkgs.writeShellApplication {
            name = "gifs";
            runtimeInputs = [pkgs.vhs];
            text = ''
              for tape in "$PRJ_ROOT"/docs/vhs/*; do
                vhs "$tape" -o "$PRJ_ROOT/docs/assets/$(basename "$tape" .tape).gif"
              done
            '';
          };
        }
      ];
    };
  };
}
