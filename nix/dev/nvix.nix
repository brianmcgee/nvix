{lib, ...}: {
  perSystem = {self', ...}: {
    config.process-compose = {
      dev.settings.processes = {
        nvix-store-init = {
          depends_on = {
            nats-server.condition = "process_healthy";
            nsc-push.condition = "process_completed_successfully";
          };
          working_dir = "$PRJ_DATA_DIR";
          environment = {
            NATS_CREDENTIALS_FILE = "./nsc/creds/Tvix/Store/Admin.creds";
          };
          command = "${lib.getExe self'.packages.nvix} store init -v";
        };
        nvix-store = {
          depends_on = {
            nvix-store-init.condition = "process_completed_successfully";
          };
          working_dir = "$PRJ_DATA_DIR";
          environment = {
            NATS_CREDENTIALS_FILE = "./nsc/creds/Tvix/Store/Server.creds";
          };
          command = "${lib.getExe self'.packages.nvix} store run -v";
          # TODO readiness probe
        };
      };
    };
  };
}
