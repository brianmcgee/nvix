{...}: {
  imports = [
    ./nats.nix
    ./nvix.nix
    ./tvix.nix
  ];

  perSystem = {self', ...}: {
    config.process-compose.dev.settings = {
      log_location = "$PRJ_DATA_DIR/dev.log";
    };

    config.devshells.default = {
      commands = [
        {
          category = "development";
          help = "Run local dev services";
          package = self'.packages.dev;
        }
        {
          category = "development";
          help = "Re-initialise state for dev services";
          name = "dev-init";
          command = "rm -rf $PRJ_DATA_DIR && direnv reload";
        }
      ];
    };
  };
}
