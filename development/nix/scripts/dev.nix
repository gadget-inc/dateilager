{ lib
, ansifilter
, coreutils
, moreutils
, services
, writeShellScriptBin
}:

let
  packages = lib.unique ([
    ansifilter
    coreutils
    moreutils
  ] ++ builtins.concatMap (service: service.packages or []) services);

  compileService = isLast: service:
    let
      colorize = text:
        "$'\\e[1;${service.ansiColor or ""}m${text}\\e[0m'";

      consoleFilter = lib.optionalString (service ? consoleFilter)
        "| grep -v ${service.consoleFilter}";
    in
      builtins.concatStringsSep "\n" ([]
        ++ lib.optional (service ? env) service.env
        ++ lib.optional (service ? setup) ''
          (${service.setup}) 2>&1 | \
            ts ${colorize "${service.name} (setup)>"}
        ''
        ++ lib.optional (service ? run) ''
          (${service.run}) 2>&1 | pee \
            "ts ${colorize "${service.name}>"}${consoleFilter}" \
            'ansifilter > tmp/log/${service.name}.log'${lib.optionalString (!isLast) " &"}
        ''
        ++ lib.optional (!(service ? run) && isLast) ''
          sleep infinity
        '');

  compileServices = services:
    builtins.concatStringsSep "\n"
      (lib.imap0
        (i: service:
          compileService (i == builtins.length services - 1) service)
        services);
in
writeShellScriptBin "dev" ''
  set -e
  export PATH=${lib.makeBinPath packages}:$PATH

  # Stop all services when Ctrl-C is pressed or error occurs
  trap 'kill $(jobs -p) 2>/dev/null; wait $(jobs -p)' INT TERM ERR EXIT

  mkdir -p tmp/log

  ${compileServices services}
'' // {
  meta = with lib; {
    description = "Runs all the services necessary for DateiLager development";
  };
}
