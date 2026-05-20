{
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.services.cc-tools-width-daemon;
in {
  options.services.cc-tools-width-daemon = {
    enable = lib.mkEnableOption "cc-tools terminal-width detection daemon (provides /dev/shm/cc-tools/parent-width for headless Claude Code agents)";

    package = lib.mkOption {
      type = lib.types.package;
      description = "The cc-tools package providing the `cc-tools` binary.";
    };

    activeInterval = lib.mkOption {
      type = lib.types.str;
      default = "1s";
      description = "Polling cadence after a recent width change (Go duration string).";
    };

    idleInterval = lib.mkOption {
      type = lib.types.str;
      default = "5s";
      description = "Polling cadence after idleAfter has elapsed without a change (Go duration string).";
    };

    idleAfter = lib.mkOption {
      type = lib.types.str;
      default = "30s";
      description = "Time without a change before backing off to idleInterval (Go duration string).";
    };

    writerDir = lib.mkOption {
      type = lib.types.str;
      default = "/dev/shm/cc-tools";
      description = "Directory where parent-width and widths.json are atomically written.";
    };

    tmuxPackage = lib.mkOption {
      type = lib.types.nullOr lib.types.package;
      default = pkgs.tmux;
      description = ''
        Package providing the `tmux` binary on the daemon's PATH. The
        daemon forks `tmux list-clients` once per tick; without this on
        PATH the tmux source is unavailable (utmp still works).
        Set to null to omit tmux from PATH entirely.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    systemd.user.services.cc-tools-width-daemon = {
      Unit = {
        Description = "cc-tools terminal-width detection daemon";
        Documentation = "https://github.com/Veraticus/cc-tools";
        After = ["default.target"];
      };

      Service = {
        ExecStart =
          "${cfg.package}/bin/cc-tools width-daemon"
          + " --active-interval=${cfg.activeInterval}"
          + " --idle-interval=${cfg.idleInterval}"
          + " --idle-after=${cfg.idleAfter}"
          + " --writer-dir=${cfg.writerDir}";
        Restart = "always";
        RestartSec = "5s";

        # The daemon forks `tmux list-clients`. systemd user services
        # start with a minimal PATH and no XDG_RUNTIME_DIR, so:
        #   - PATH must include tmux or the binary isn't found
        #   - XDG_RUNTIME_DIR must point at the user's runtime dir
        #     because modern tmux puts its socket at
        #     $XDG_RUNTIME_DIR/tmux-$UID/default rather than the
        #     legacy /tmp/tmux-$UID/default. Without it, tmux exits
        #     "no server running" and we miss tmux as a source.
        # %U is systemd's specifier for the numeric UID.
        Environment =
          (lib.optional (cfg.tmuxPackage != null) "PATH=${lib.makeBinPath [cfg.tmuxPackage]}")
          ++ ["XDG_RUNTIME_DIR=/run/user/%U"];

        # Light hardening — the daemon doesn't need much. It reads
        # /var/run/utmp and forks tmux; that's it.
        NoNewPrivileges = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        PrivateTmp = false; # daemon writes to /dev/shm, which must be the real one
        RestrictRealtime = true;
      };

      Install.WantedBy = ["default.target"];
    };
  };
}
