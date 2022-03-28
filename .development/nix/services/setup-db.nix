{}:

{
  name = "setup-db";
  ansiColor = "36";

  setup = ''
    wait_for_postgres

    for dbname in dl dl_tests; do
      if ! $(database_exists $dbname); then
        echo "== Creating '$dbname' database =="
        create_database $dbname
      fi
    done
  '';
}
