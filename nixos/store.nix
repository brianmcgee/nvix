{
  lib,
  pkgs,
  config,
  ...
}: let
  cfg = config.services.nvix.store;
in {
  options.services.nvix.store = with lib; {
    enable = mkEnableOption (mdDoc "Enable NVIX Store");
    package = mkOption {
      type = types.package;
      default = pkgs.nvix;
      defaultText = literalExpression "pkgs.nvix";
      description = mdDoc "Package to use for nits.";
    };
    listen = {
      address = mkOption {
        type = types.str;
        default = "localhost:5000";
        description = "interface and port to listen on";
      };
    };
    metrics = {
      address = mkOption {
        type = types.str;
        default = "localhost:5050";
        description = "interface and port to listen on";
      };
    };
    nats = {
      url = mkOption {
        type = types.str;
        example = "nats://localhost:4222";
        description = mdDoc "NATS server url.";
      };
      credentialsFile = mkOption {
        type = types.path;
        example = "/mnt/shared/user.creds";
        description = mdDoc "Path to a file containing a NATS credentials file";
      };
    };
    verbosity = mkOption {
      type = types.int;
      default = 1;
      example = "2";
      description = mdDoc "Selects the log verbosity.";
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.services.nvix-store = {
      after = ["network.target"];
      wantedBy = ["sysinit.target"];

      description = "NVIX Store";
      startLimitIntervalSec = 0;

      # the agent will restart itself after a successful deployment
      restartIfChanged = false;

      environment = lib.filterAttrs (_: v: v != null) {
        NATS_URL = cfg.nats.url;
        LISTEN_ADDRESS = cfg.listen.address;
        METRICS_ADDRESS = cfg.metrics.address;
        LOG_LEVEL = "${builtins.toString cfg.verbosity}";
      };

      serviceConfig = with lib; {
        Restart = mkDefault "on-failure";
        RestartSec = 1;

        LoadCredential = [
          "nats.creds:${cfg.nats.credentialsFile}"
        ];

        User = "nvix-store";
        DynamicUser = true;
        StateDirectory = "nvix-store";
        ExecStart = "${cfg.package}/bin/nvix store run --nats-credentials %d/nats.creds";
      };
    };
  };
}
