{inputs, ...}: {
  perSystem = {
    pkgs,
    system,
    ...
  }: let
    depot = import inputs.depot {
      nixpkgsBisectPath = pkgs.path;
      localSystem = system;
    };
  in {
    config.devshells.default = {
      env = [
        {
          name = "TVIX_HOME";
          eval = "$PRJ_DATA_DIR/tvix";
        }
        {
          name = "BLOB_SERVICE_ADDR";
          value = "grpc+http://localhost:5000";
        }
        {
          name = "PATH_INFO_SERVICE_ADDR";
          eval = "sled://$TVIX_HOME/store/path-info";
        }
        {
          name = "DIRECTORY_SERVICE_ADDR";
          eval = "sled://$TVIX_HOME/store/directory";
        }
      ];

      commands = let
        category = "tvix";
      in [
        {
          inherit category;
          name = "tvix";
          package = depot.tvix.cli;
        }
        {
          inherit category;
          name = "tvix-store";
          package = depot.tvix.store;
        }
      ];
    };
  };
}
