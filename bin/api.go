package main

import (
	//ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	//"github.com/kr/pretty"

	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http/cgi"
	"os"
	"regexp"
	"runtime"
	"strings"
)

type MyReadCloser struct {
	r *bytes.Buffer
}

type Request struct {
	Corpus string `json:"corpus"`
	Query  string `json:"query"`
	Want   string `json:"want"` // csv -> tabel, text -> id + zin, json, xml
	Mark   string `json:"mark"` // woordmarkering voor want=text: none, ansi, text
}

var (
	reComment = regexp.MustCompile(`(?s:/\*.*?\*/)|--.*`)
	db        *sql.DB
	ctx       context.Context
	cancel    context.CancelFunc
	hasCancel = false

	chRow  = make(chan []interface{})
	chErr  = make(chan error)
	chQuit = make(chan bool)
	chDone = make(chan bool)
)

func main() {
	req, err := cgi.Request()
	if err != nil {
		errout(err)
		return
	}

	body := []byte{}

	if req.Body != nil {
		var err error
		body, err = ioutil.ReadAll(req.Body)
		if err != nil {
			errout(err)
			return
		}
		req.Body.Close()
		var b bytes.Buffer
		b.Write(body)
		req.Body = MyReadCloser{r: &b}
	}

	var rq Request

	if len(body) > 0 && body[0] == '{' {
		err := json.Unmarshal(body, &rq)
		if err != nil {
			errout(err)
			return
		}
	} else {
		rq.Corpus = req.FormValue("corpus")
		rq.Query = req.FormValue("query")
	}

	rq.Corpus = strings.TrimSpace(strings.Replace(rq.Corpus, "'", "", -1))
	if rq.Corpus == "" {
		errout(fmt.Errorf("Missing corpus"))
		return
	}

	rq.Query, err = safeQuery(rq.Query)
	if err != nil {
		errout(err)
		return
	}

	err = openDB()
	if err != nil {
		errout(err)
		return
	}

	go doQuery(rq)

	go doRows(rq)

	select {
	case err := <-chErr:
		errout(err)
		return
	case <-chDone:
		return
	case <-chQuit:
		return
	}

}

func safeQuery(query string) (string, error) {

	// TODO: is dit nog nodig? JA!
	// https://github.com/bitnine-oss/agensgraph/issues/496
	query = strings.Replace(query, ":'", ": '", -1)

	// verwijder alle separators
	query = strings.TrimSpace(strings.Replace(query, ";", " ", -1))

	query = reComment.ReplaceAllLiteralString(query, "")
	query = strings.TrimSpace(query)

	qu := strings.ToUpper(query)
	if !(strings.HasPrefix(qu, "MATCH") || strings.HasPrefix(qu, "SELECT")) {
		return "", fmt.Errorf("Query must start with MATCH or SELECT")
	}

	return query, nil

}

func doQuery(rq Request) {
	defer func() {
		if r := recover(); r != nil {
			chErr <- fmt.Errorf("Recovered in doQuery: %v", r)
		}
		close(chRow)
	}()

	rows, err := db.QueryContext(ctx, qc(rq.Corpus, rq.Query))
	if err != nil {
		chErr <- wrap(err)
		return
	}

	ctypes, _ := rows.ColumnTypes()

	for rows.Next() {
		select {
		case <-chQuit:
			break
		default:
		}
		scans := make([]interface{}, len(ctypes))
		for i := range ctypes {
			scans[i] = new([]uint8)
		}
		err := rows.Scan(scans...)
		if err != nil {
			chErr <- wrap(err)
			return
		}
		chRow <- scans
	}
}

func doRows(rq Request) {
	defer func() {
		close(chDone)
	}()

	switch rq.Want {
	case "xml":
		fmt.Printf("Content-type: text/xml; charset=utf-8\n\n<?xml version=\"1.0\"?>\n<rows>\n")
	case "json":
		fmt.Printf("Content-type: text/plain; charset=utf-8\n\n[\n")
	case "text":
		fmt.Printf("Content-type: text/plain; charset=utf-8\n\n")
	default:
		fmt.Printf("Content-type: text/csv; charset=utf-8\n\n")
	}

	started := false
	sep := ""
	for row := range chRow {
		fmt.Print(sep)
		if !started {
			started = true
			if rq.Want == "json" {
				sep = ",\n"
			}
		}

		switch rq.Want {
		case "xml":
			doXML(row)
		case "json":
			doJSON(row)
		case "text":
			doText(row, rq.Mark)
		default:
			doCSV(row)
		}
	}

	switch rq.Want {
	case "xml":
		fmt.Printf("</rows>\n")
	case "json":
		fmt.Printf("\n]\n")
	}
}

func qc(corpus, query string) string {
	return fmt.Sprintf("set graph_path='%s';\n%s", corpus, query)
}

func doXML(row []interface{}) {
	fmt.Println("<row>\n<cols>")
	for _, v := range row {
		val := *(v.(*[]byte))
		fmt.Println("<col>" + html.EscapeString(string(val)) + "</col>")
	}
	fmt.Println("</cols>")

	// TODO

	fmt.Println("</row>")

}

func doJSON(row []interface{}) {
	fmt.Print("{\"cols\": [")
	for i, v := range row {
		if i > 0 {
			fmt.Print(",")
		}
		val := *(v.(*[]byte))
		fmt.Printf("%q", string(val))
	}
	fmt.Print("]")

	// TODO

	fmt.Print("}")
}

func doText(row []interface{}, mark string) {
	// TODO
}

func doCSV(row []interface{}) {
	for i, v := range row {
		if i > 0 {
			fmt.Print(",")
		}
		val := *(v.(*[]byte))
		sval := `"` + strings.Replace(string(val), `"`, `""`, -1) + `"`
		if n := len(sval); n >= 6 && strings.HasPrefix(sval, `"""`) && strings.HasSuffix(sval, `"""`) {
			sval = sval[2 : n-2]
		}
		fmt.Print(sval)
	}
	fmt.Println()
}

func openDB() error {

	var login string
	if strings.HasPrefix(os.Getenv("CONTEXT_DOCUMENT_ROOT"), "/home/peter") {
		login = "port=9333 user=peter dbname=peter sslmode=disable"
	} else {
		login = "user=guest password=guest port=19033 dbname=p209327 sslmode=disable"
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

	ctx, cancel = context.WithCancel(context.Background())
	hasCancel = true

	return nil
}

func wrap(err error) error {
	_, filename, lineno, ok := runtime.Caller(1)
	if ok {
		return fmt.Errorf("%v:%v: %v", filename, lineno, err)
	}
	return err
}

func errout(err error) {
	fmt.Printf("Content-type: text/plain; charset=utf-8\n\nError: %v\n", err)
}

func (rc MyReadCloser) Close() error {
	return nil
}

func (rc MyReadCloser) Read(p []byte) (n int, err error) {
	return rc.Read(p)
}
