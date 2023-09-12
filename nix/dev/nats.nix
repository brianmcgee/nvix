{lib, ...}: {
  perSystem = {pkgs, ...}: let
    config = pkgs.writeTextFile {
      name = "nats.conf";
      text = ''
        ## Default NATS server configuration (see: https://docs.nats.io/running-a-nats-service/configuration)

        ## Host for client connections.
        host: "127.0.0.1"

        ## Port for client connections.
        port: 4222

        ## Port for monitoring
        http_port: 8222

        ## Increase max payload to 8MB
        max_payload: 8388608

        ## Configuration map for JetStream.
        ## see: https://docs.nats.io/running-a-nats-service/configuration#jetstream
        jetstream {}

        # include paths must be relative so for simplicity we just read in the auth.conf file
        include './auth.conf'
      '';
    };

    # we need to wrap nsc and nats to ensure they pick up key related stated from the data directory
    nscWrapped = pkgs.writeShellScriptBin "nsc" ''
      XDG_CONFIG_HOME="$PRJ_DATA_DIR" ${pkgs.nsc}/bin/nsc -H $NSC_HOME "$@"
    '';

    natsWrapped = pkgs.writeShellScriptBin "nats" ''
      XDG_CONFIG_HOME="$PRJ_DATA_DIR" ${pkgs.natscli}/bin/nats "$@"
    '';
  in {
    config.process-compose = {
      dev.settings.processes = {
        nats-server = {
          working_dir = "$NATS_HOME";
          command = ''${lib.getExe pkgs.nats-server} -c ./nats.conf -sd ./'';
          readiness_probe = {
            http_get = {
              host = "127.0.0.1";
              port = 8222;
              path = "/healthz";
            };
            initial_delay_seconds = 2;
          };
        };

        nsc-push = {
          depends_on = {
            nats-server.condition = "process_healthy";
          };
          environment = {
            XDG_CONFIG_HOME = "$PRJ_DATA_DIR";
          };
          command = pkgs.writeShellApplication {
            name = "nsc-push";
            runtimeInputs = [nscWrapped];
            text = ''nsc push'';
          };
        };
      };
    };

    config.devshells.default = {
      env = [
        {
          name = "NATS_HOME";
          eval = "$PRJ_DATA_DIR/nats";
        }
        {
          name = "NSC_HOME";
          eval = "$PRJ_DATA_DIR/nsc";
        }
        {
          name = "NKEYS_PATH";
          eval = "$NSC_HOME";
        }
        {
          name = "NATS_JWT_DIR";
          eval = "$PRJ_DATA_DIR/nats/jwt";
        }
      ];

      devshell.startup = {
        setup-nats.text = ''
          set -euo pipefail

          # we only setup the data dir if it doesn't exist
          # to refresh simply delete the directory and run `direnv reload`
          [ -d $NSC_HOME ] && exit 0

          mkdir -p $NSC_HOME

          # initialise nsc state

          nsc init -n Tvix --dir $NSC_HOME
          nsc edit operator \
              --service-url nats://localhost:4222 \
              --account-jwt-server-url nats://localhost:4222

          # setup server config

          mkdir -p $NATS_HOME
          cp ${config} "$NATS_HOME/nats.conf"
          nsc generate config --nats-resolver --config-file "$NATS_HOME/auth.conf"

          nsc add account -n Store
          nsc edit account -n Store \
              --js-mem-storage -1 \
              --js-disk-storage -1 \
              --js-streams -1 \
              --js-consumer -1

          nsc add user -a Store -n Admin
          nsc add user -a Store -n Server
        '';
      };

      packages = [
        pkgs.nkeys
        pkgs.nats-top
      ];

      commands = let
        category = "nats";
      in [
        {
          inherit category;
          help = "Creates NATS operators, accounts, users, and manage their permissions";
          package = nscWrapped;
        }
        {
          inherit category;
          help = "NATS Server and JetStream administration";
          package = natsWrapped;
        }
      ];
    };
  };
}
