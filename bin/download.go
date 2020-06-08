package main

import (
	_ "github.com/lib/pq"
	"github.com/rug-compling/alud/v2"

	"bytes"
	"database/sql"
	"fmt"
	"net/http/cgi"
	"os"
	"runtime"
	"strings"
)

var (
	db *sql.DB
)

func main() {

	req, err := cgi.Request()
	x(err)

	x(req.ParseForm())

	corpus := strings.Replace(req.FormValue("corpus"), "'", "", -1)
	sentid := strings.Replace(req.FormValue("sentid"), "'", "", -1)
	want := strings.TrimSpace(req.FormValue("want"))

	x(openDB())

	_, err = db.Exec("set graph_path='" + corpus + "'")
	x(err)

	var alpino string
	if strings.Contains(want, "Alpino") {
		alpino = cyp2alp(sentid)
	}

	var ud string
	if strings.Contains(want, "UD") {
		cp := corpus
		if c, ok := corpora[cp]; ok {
			cp = c
		}
		ud = cyp2ud(cp, sentid, alpino == "")
	}

	if alpino != "" && ud != "" {
		var auto string
		rows, err := db.Query("match (d:doc) return d.alud_version limit 1")
		x(err)
		for rows.Next() {
			x(rows.Scan(&auto))
		}
		x(rows.Err())

		alpino, err = alud.Alpino([]byte(alpino), ud, unescape(auto))
		x(err)
		alpino += "\n"
	}

	if alpino != "" {
		fmt.Printf(`Content-type: text/xml; charset=utf-8
Content-Disposition: attachment; filename=%s.xml

%s`, sentid, alpino)
	} else {
		fmt.Printf(`Content-type: text/plain; charset=utf-8
Content-Disposition: attachment; filename=%s.txt

%s`, sentid, ud)
	}
}

func openDB() error {

	var login string
	if s := os.Getenv("CONTEXT_DOCUMENT_ROOT"); strings.HasPrefix(s, "/home/peter") {
		login = "port=9333 user=peter dbname=peter sslmode=disable"
	} else if strings.HasPrefix(s, "/var/www/html") {
		login = "port=5432 user=guest password=guest dbname=user sslmode=disable"
	} else {
		login = "port=19033 user=guest password=guest dbname=p209327 sslmode=disable"
		if h, _ := os.Hostname(); !strings.HasPrefix(h, "haytabo") {
			login += " host=haytabo.let.rug.nl"
		}
	}

	var err error
	db, err = sql.Open("postgres", login)
	if err != nil {
		return err
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		return err
	}

	return nil
}

func x(err error, msg ...interface{}) {
	if err == nil {
		return
	}

	var b bytes.Buffer
	_, filename, lineno, ok := runtime.Caller(1)
	if ok {
		b.WriteString(fmt.Sprintf("%v:%v: %v", filename, lineno, err))
	} else {
		b.WriteString(err.Error())
	}
	if len(msg) > 0 {
		b.WriteString(",")
		for _, m := range msg {
			b.WriteString(fmt.Sprintf(" %v", m))
		}
	}
	fmt.Print(`Content-type: text/plain; charset=utf-8

`)

	fmt.Println(b.String())
	os.Exit(0)
}
