{lib, ...}: {
  perSystem = {self', ...}: {
    config.process-compose = {
      dev.settings.processes = {
        nvix-store = {
          depends_on = {
            nats-server.condition = "process_healthy";
            nsc-push.condition = "process_completed_successfully";
          };
          working_dir = "$PRJ_DATA_DIR";
          environment = {
            NVIX_STORE_NATS_CREDENTIALS_FILE = "./nsc/creds/Tvix/Store/Server.creds";
          };
          command = lib.getExe self'.packages.nvix;
          # TODO readiness probe
        };
      };
    };
  };
}
