package main

import (
	ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http/cgi"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

type MyReadCloser struct {
	r *bytes.Buffer
}

type Request struct {
	Corpus   string `json:"corpus"`
	Query    string `json:"query"`
	Want     string `json:"want"` // tsv -> tabel, text -> id + zin, json, xml
	Mark     string `json:"mark"` // woordmarkering voor want=text: none, ansi, text
	wantMark bool
}

type RowT struct {
	XMLName  xml.Name `json:"-"                   xml:"row"`
	Cols     []string `json:"values"              xml:"values>v"`
	Sentence string   `json:"sentence,omitempty"  xml:"sentence,omitempty"`
	SentID   string   `json:"sentid,omitempty"    xml:"sentid,omitempty"`
	Marks    []int    `json:"marks,omitempty      xml:"marks>m,omitempty"`
}

var (
	reQuote   = regexp.MustCompile(`\\.`)
	reComment = regexp.MustCompile(`(?s:/\*.*?\*/)|--.*`)
	db        *sql.DB
	ctx       context.Context
	cancel    context.CancelFunc
	hasCancel = false

	headers []string

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
	headers = make([]string, len(ctypes))
	for i, ct := range ctypes {
		headers[i] = ct.Name()
	}

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
		rq.wantMark = true
	case "json":
		fmt.Printf("Content-type: text/plain; charset=utf-8\n\n{\n  \"rows\": [\n")
		rq.wantMark = true
	case "text":
		fmt.Printf("Content-type: text/plain; charset=utf-8\n\n")
		rq.wantMark = rq.Mark == "text" || rq.Mark == "ansi"

	default:
		fmt.Printf("Content-type: text/tab-separated-values; charset=utf-8\n\n")
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
			doXML(row, rq)
		case "json":
			doJSON(row, rq)
		case "text":
			doText(row, rq)
		default:
			doTSV(row, rq)
		}
	}

	switch rq.Want {
	case "xml":
		fmt.Printf("</rows>\n")
	case "json":
		fmt.Printf("\n  ]\n}\n")
	}
}

func qc(corpus, query string) string {
	return fmt.Sprintf("set graph_path='%s';\n%s", corpus, query)
}

func doXML(row []interface{}, rq Request) {
	r := doRow(row, rq)
	b, err := xml.MarshalIndent(r, "  ", "  ")
	if err != nil {
		chErr <- wrap(err)
		return
	}
	fmt.Println(string(b))

}

func doJSON(row []interface{}, rq Request) {
	r := doRow(row, rq)
	b, err := json.MarshalIndent(r, "    ", "  ")
	if err != nil {
		chErr <- wrap(err)
		return
	}
	fmt.Print("    " + string(b))
}

func doText(row []interface{}, rq Request) {
	r := doRow(row, rq)
	if rq.wantMark {
		p1 := "[[ "
		p2 := " ]]"
		if rq.Mark == "ansi" {
			p1 = "\x1B[7m"
			p2 = "\x1B[0m"
		}
		words := strings.Fields(r.Sentence)
		n := len(words)
		if p := r.Marks[0]; p > 0 && p <= n {
			words[p-1] = p1 + words[p-1]
		}
		if p := r.Marks[len(r.Marks)-1]; p > 0 && p <= n {
			words[p-1] = words[p-1] + p2
		}
		for i, p := range r.Marks[:len(r.Marks)-1] {
			if p >= 0 && p < n {
				if p+1 != r.Marks[i+1] {
					words[p-1] = words[p-1] + p2
				}
			}
		}
		for i, p := range r.Marks[1:len(r.Marks)] {
			if p-1 != r.Marks[i] {
				words[p-1] = p1 + words[p-1]
			}
		}
		r.Sentence = strings.Join(words, " ")
	}
	fmt.Printf("%s\t%s\n", r.SentID, r.Sentence)
}

func doTSV(row []interface{}, rq Request) {
	for i, v := range row {
		if i > 0 {
			fmt.Print("\t")
		}
		val := *(v.(*[]byte))
		fmt.Print(string(val))
	}
	fmt.Println()
}

func doRow(row []interface{}, rq Request) *RowT {
	rt := RowT{
		Cols: make([]string, len(row)),
	}

	ids := make(map[int]bool)

	for i, v := range row {
		val := *(v.(*[]byte))
		sval := string(val)
		rt.Cols[i] = sval
		if headers[i] == `sentid` {
			rt.SentID = unescape(sval)
		} else if headers[i] == `id` {
			n, err := strconv.Atoi(unescape(sval))
			if err == nil {
				ids[n] = true
			}
		}
		isPath := false
		if strings.HasPrefix(sval, "[sentence") ||
			strings.HasPrefix(sval, "[node") ||
			strings.HasPrefix(sval, "[word") ||
			strings.HasPrefix(sval, "[meta") ||
			strings.HasPrefix(sval, "[doc") {
			var p ag.BasicPath
			if p.Scan(val) == nil {
				isPath = true
				for _, v := range p.Vertices {
					if sid, ok := v.Properties["sentid"]; ok {
						rt.SentID = fmt.Sprint(sid)
					}
					if id, ok := v.Properties["id"]; ok {
						if iid, err := strconv.Atoi(fmt.Sprint(id)); err == nil {
							ids[iid] = true
						}
					}
				}
			}
		}
		if !isPath {
			var v ag.BasicVertex
			if v.Scan(val) == nil {
				if sid, ok := v.Properties["sentid"]; ok {
					rt.SentID = fmt.Sprint(sid)
				}
				if id, ok := v.Properties["id"]; ok {
					if iid, err := strconv.Atoi(fmt.Sprint(id)); err == nil {
						ids[iid] = true
					}
				}
			}
		}
	}

	if rt.SentID == "" {
		return &rt
	}

	rows, err := db.QueryContext(ctx, qc(rq.Corpus, "match (s:sentence{sentid: '"+safeString(rt.SentID)+"'}) return s.tokens"))
	if err != nil {
		chErr <- wrap(err)
		return &rt
	}
	for rows.Next() {
		var s string
		err := rows.Scan(&s)
		if err != nil {
			chErr <- wrap(err)
			return &rt
		}
		rt.Sentence = unescape(s)
	}

	if !rq.wantMark || len(ids) == 0 {
		return &rt
	}

	rt.Marks = make([]int, 0)

	idlist := make([]string, 0, len(ids))
	for key := range ids {
		idlist = append(idlist, fmt.Sprint(key))
	}

	rows, err = db.QueryContext(
		ctx,
		qc(rq.Corpus, fmt.Sprintf(
			"match (n:nw{sentid: '%s'})-[:rel*0..]->(w:word{sentid: '%s'}) where n.id in [%s] return distinct w.end as p order by p",
			rt.SentID, rt.SentID, strings.Join(idlist, ","))))
	if err != nil {
		chErr <- wrap(err)
		return &rt
	}
	for rows.Next() {
		var end int
		err := rows.Scan(&end)
		if err != nil {
			chErr <- wrap(err)
			return &rt
		}
		rt.Marks = append(rt.Marks, end)
	}
	if err := rows.Err(); err != nil {
		chErr <- wrap(err)
	}

	return &rt
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

func safeString(s string) string {
	return strings.Replace(strings.Replace(s, "'", "", -1), `\`, "", -1)
}

func unescape(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}

	s = s[1 : len(s)-1]
	return reQuote.ReplaceAllStringFunc(s, func(s1 string) string {
		if s1 == `\n` {
			return "\n"
		}
		if s1 == `\r` {
			return "\r"
		}
		if s1 == `\t` {
			return "\t"
		}
		return s1[1:]
	})
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
