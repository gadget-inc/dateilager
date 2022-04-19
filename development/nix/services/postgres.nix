{ coreutils
, postgresql
}:

{
  name = "postgres";
  ansiColor = "34";

  packages = [
    coreutils # sleep
    postgresql
  ];

  env = ''
    export PGDATA="$PWD/tmp/postgres"
    export PGUSER=postgres
    export PGPASSWORD=password
    export PGURI="postgres://$PGUSER:$PGPASSWORD@127.0.0.1"

    wait_for_postgres() {
      until psql "$PGURI/postgres" -c '\q' 2> /dev/null; do
        sleep 0.2
      done
    }

    database_exists() {
      if [ "$(psql "$PGURI/postgres" -qtAc "SELECT 1 FROM pg_database WHERE datname = '$1'")" == "1" ]; then
        exit 0
      else
        exit 1
      fi
    }

    create_database() {
      psql "$PGURI/postgres" -c "CREATE DATABASE $dbname;"
    }
  '';

  setup = ''
    if [ ! -d "$PGDATA" ]; then
      echo "== Creating postgres database cluster =="
      initdb --username="$PGUSER" --pwfile=<(echo "$PGPASSWORD")
    fi
  '';

  # Don't try to create a unix socket
  # We only use TCP sockets and some systems require root permissions for unix sockets
  run = ''
    postgres -c unix_socket_directories= -c timezone=UTC -c fsync=off -c synchronous_commit=off -c full_page_writes=off
  '';
}
