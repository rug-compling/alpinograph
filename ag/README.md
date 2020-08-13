
## Globale settings, voor elke sessie

    PATH={PATH/TO}/opt/agensgraph/bin:$PATH
    export PGPORT={PORTNUMBER}
    export AGDATA={PATH/TO}/var/agensgraph/db_cluster


## Eenmalig

Database aanmaken, gebruik **niet** je loginnaam:

    initdb -A md5 --locale=en_US.utf8 -U {ROOTUSERNAME} -W

Poortnummer aanpassen in: {PATH/TO}/`var/agensgraph/db_cluster/postgresql.conf`

In {PATH/TO}/`var/agensgraph/db_cluster/pg_hba.conf` regels toevoegen
voor elke gebruiker die van buiten verbinding mag maken.

Starten, en reguliere gebruiker aanmaken:

    ag_ctl start

    createuser -d -e -U {ROOTUSERNAME} -W `id -un` -P

    createdb -e -O `id -un` -U {ROOTUSERNAME} -W


## Per sessie

Starten database:

    ag_ctl start

Stoppen database:

    ag_stl stop


## Invoer corpus

    export PGPASSWORD={USERPASSWORD}
    ./alpino2agens {CORPUSNAME}.dact | agens -b -q


## Dump corpus

    pg_dump `id -un` -n {CORPUSNAME}

## Command-line interface

Starten:

    agens

Enkele commando's in AgensGraph:

    set graph_path = '{CORPUSNAME}';

    match (w:word) return w limit 2;

Lijst van corpora (`public` is geen corpus):

    \dn

Gegevens van een corpus:

    select
        tablename,
        indexname,
        indexdef
    from
        pg_indexes
    where
        schemaname = '{CORPUSNAME}'
    order by
        tablename,
        indexname;
