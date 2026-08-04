{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.boblox;

  # Build a customized Python environment containing all necessary packages for RFD.
  pythonEnv = pkgs.python3.withPackages (ps: with ps; [
    pygobject3
    websocket-client
    requests
    trustme
    urllib3
    pyzstd
    py7zr
    lz4
    # dracopy # Uncomment if available in your nixpkgs channel
  ]);
in {
  options.services.boblox = {
    enable = mkEnableOption "Boblox Headless Server (rbxdserver)";

    package = mkOption {
      type = types.package;
      default = pkgs.callPackage ./package.nix {};
      defaultText = literalExpression "pkgs.callPackage ./package.nix {}";
      description = "The rbxdserver package to run.";
    };

    port = mkOption {
      type = types.port;
      default = 8080;
      description = "Port for the HTTP/WS API.";
    };

    placesDir = mkOption {
      type = types.path;
      default = "/var/lib/boblox/places";
      description = "Directory where the Roblox places are stored.";
    };

    rfdDir = mkOption {
      type = types.path;
      default = "/var/lib/boblox/rfd";
      description = "Directory where the unified rfd-fork (containing Source/ and Roblox/) is located.";
    };

    dataDir = mkOption {
      type = types.path;
      default = "/var/lib/boblox";
      description = "Directory for daemon state, logs, and sessions.";
    };

    tokenFile = mkOption {
      type = types.path;
      default = "/run/secrets/boblox-token";
      description = "Path to the file containing the auth token. MUST NOT be in the Nix store.";
    };

    user = mkOption {
      type = types.str;
      default = "boblox";
      description = "User account under which rbxdserver runs.";
    };

    group = mkOption {
      type = types.str;
      default = "boblox";
      description = "Group under which rbxdserver runs.";
    };

    openFirewall = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to open the API port in the firewall.";
    };

    testMode = mkOption {
      type = types.bool;
      default = false;
      description = "Whether to run the server in test mode (disables headless cage rendering).";
    };

    extraEnv = mkOption {
      type = types.attrsOf types.str;
      default = {};
      description = "Extra environment variables to pass to the rbxdserver service.";
    };
  };

  config = mkIf cfg.enable {
    users.users.${cfg.user} = {
      isSystemUser = true;
      group = cfg.group;
      home = cfg.dataDir;
      createHome = true;
      description = "Boblox (rbxdserver) service user";
    };

    users.groups.${cfg.group} = {};

    systemd.services.boblox = {
      description = "Boblox Headless Server (rbxdserver)";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

      # Provide runtime dependencies to the service's PATH
      path = with pkgs; [
        pythonEnv
        cage
        umu-launcher
        bash
        coreutils
      ];

      serviceConfig = {
        Type = "simple";
        User = cfg.user;
        Group = cfg.group;
        WorkingDirectory = cfg.dataDir;
      
        ExecStart = "${cfg.package}/bin/rbxdserver --port ${toString cfg.port} --rfd ${cfg.rfdDir} --places ${cfg.placesDir} --state-dir ${cfg.dataDir} --token-file ${cfg.tokenFile}${optionalString cfg.testMode " --test"}";
        
        Restart = "on-failure";
        RestartSec = "5s";

        # Sandboxing & security settings (tailored to not block Wine / UMU)
        NoNewPrivileges = true;
        ProtectHome = "read-only";

        RuntimeDirectory = "boblox";
      };

      environment = {
        XDG_RUNTIME_DIR = "/run/boblox";
      } // cfg.extraEnv;
    };

    networking.firewall.allowedTCPPorts = mkIf cfg.openFirewall [ cfg.port ];
  };
}
