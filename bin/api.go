package main

import (
	ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http/cgi"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	LIMIT = 100000
)

type MyReadCloser struct {
	r *bytes.Buffer
}

type Request struct {
	Corpus   string `json:"corpus"`
	Query    string `json:"query"`
	Want     string `json:"want"` // tsv -> tabel, text -> id + zin, json, xml, attr
	Mark     string `json:"mark"` // woordmarkering voor want=text: none, ansi, text
	Attr     string `json:"attr"` // word, lemma, ...
	Limit    int    `json:"limit"`
	wantMark bool
}

type RowT struct {
	XMLName  xml.Name `json:"-"                   xml:"row"`
	Cols     []string `json:"values"              xml:"values>v"`
	Sentence string   `json:"sentence,omitempty"  xml:"sentence,omitempty"`
	SentID   string   `json:"sentid,omitempty"    xml:"sentid,omitempty"`
	Marks    []int    `json:"marks,omitempty"     xml:"marks>m,omitempty"`
	attribs  []string
}

type AttrT struct {
	n int
	s string
}

var (
	reQuote   = regexp.MustCompile(`\\.`)
	reComment = regexp.MustCompile(`(?s:/\*.*?\*/)|--.*`)
	db        *sql.DB

	headers []string

	chRow  = make(chan []interface{})
	chErr  = make(chan error)
	chQuit = make(chan bool)
	chDone = make(chan bool)

	attrMap  = make(map[string]int)
	attrSeen = make(map[string]bool)
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
		rq.Want = req.FormValue("want")
		rq.Mark = req.FormValue("mark")
		rq.Attr = req.FormValue("attr")
		rq.Limit, _ = strconv.Atoi(req.FormValue("limit"))
	}

	if rq.Limit < 1 || rq.Limit > LIMIT || rq.Want == "attr" {
		rq.Limit = LIMIT
	}

	if rq.Attr == "" {
		rq.Attr = "word"
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

	rows, err := db.Query(qc(rq.Corpus, rq.Query))
	if err != nil {
		chErr <- wrap(err)
		return
	}

	ctypes, _ := rows.ColumnTypes()
	headers = make([]string, len(ctypes))
	for i, ct := range ctypes {
		headers[i] = ct.Name()
	}

	count := 0
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
		count++
		if count == rq.Limit {
			break
		}
	}
}

func doRows(rq Request) {
	defer func() {
		close(chDone)
	}()

	switch rq.Want {
	case "xml":
		Printf(`Content-type: text/xml; charset=utf-8
Content-Disposition: attachment; filename=ag_data.xml

<?xml version="1.0"?>
<rows>
`)
		rq.wantMark = true
	case "json":
		Printf(`Content-type: text/plain; charset=utf-8
Content-Disposition: attachment; filename=ag_data.json

{
  "rows": [
`)
		rq.wantMark = true
	case "text":
		Printf(`Content-type: text/tab-separated-values; charset=utf-8
Content-Disposition: attachment; filename=ag_data.tsv

`)
		rq.wantMark = rq.Mark == "text" || rq.Mark == "ansi"

	case "attr":
		Printf(`Content-type: text/tab-separated-values; charset=utf-8
Content-Disposition: attachment; filename=ag_data.tsv

`)
		rq.wantMark = true
	default:
		Printf(`Content-type: text/tab-separated-values; charset=utf-8
Content-Disposition: attachment; filename=ag_data.tsv

`)
	}

	started := false
	sep := ""
	for row := range chRow {
		Print(sep)
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
		case "attr":
			doAttr(row, rq)
		default:
			doTSV(row, rq)
		}
	}

	switch rq.Want {
	case "xml":
		Printf("</rows>\n")
	case "json":
		Printf("\n  ]\n}\n")
	case "attr":
		outputAttr()
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
	Println(strings.Replace(string(b), "    <marks></marks>\n", "", 1))
}

func doJSON(row []interface{}, rq Request) {
	r := doRow(row, rq)
	b, err := json.MarshalIndent(r, "    ", "  ")
	if err != nil {
		chErr <- wrap(err)
		return
	}
	Print("    " + string(b))
}

func doText(row []interface{}, rq Request) {
	r := doRow(row, rq)
	if rq.wantMark && len(r.Marks) > 0 {
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
	Printf("%s\t%s\n", r.SentID, r.Sentence)
}

func doTSV(row []interface{}, rq Request) {
	for i, v := range row {
		if i > 0 {
			Print("\t")
		}
		val := *(v.(*[]byte))
		Print(string(val))
	}
	Println()
}

func doAttr(row []interface{}, rq Request) {
	r := doRow(row, rq)
	if len(r.Marks) == 0 {
		return
	}

	k := fmt.Sprint(r.SentID, "\t", r.Marks)
	if attrSeen[k] {
		return
	}
	attrSeen[k] = true

	ss := []string{unescape(r.attribs[0])}
	for i := 1; i < len(r.Marks); i++ {
		if r.Marks[i] != r.Marks[i-1]+1 {
			ss = append(ss, "[...]")
		}
		ss = append(ss, unescape(r.attribs[i]))
	}
	s := strings.Join(ss, " ")
	attrMap[s] = attrMap[s] + 1
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

	rows, err := db.Query(qc(rq.Corpus, "match (s:sentence{sentid: '"+safeString(rt.SentID)+"'}) return s.tokens"))
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
	rt.attribs = make([]string, 0)

	idlist := make([]string, 0, len(ids))
	for key := range ids {
		idlist = append(idlist, fmt.Sprint(key))
	}

	rows, err = db.Query(
		qc(rq.Corpus, fmt.Sprintf(
			"match (n:nw{sentid: '%s'})-[:rel*0..]->(w:word) where n.id in [%s] return distinct w.end as p, w.\"%s\" order by p",
			rt.SentID, strings.Join(idlist, ","), strings.Replace(rq.Attr, `"`, `""`, -1))))
	if err != nil {
		chErr <- wrap(err)
		return &rt
	}
	for rows.Next() {
		var end int
		var attr sql.NullString
		err := rows.Scan(&end, &attr)
		if err != nil {
			chErr <- wrap(err)
			return &rt
		}
		var a string
		if attr.Valid {
			a = attr.String
		}
		if a == "" {
			a = "NULL"
		}
		rt.Marks = append(rt.Marks, end)
		rt.attribs = append(rt.attribs, a)
	}
	if err := rows.Err(); err != nil {
		chErr <- wrap(err)
	}

	return &rt
}

func outputAttr() {
	aa := make([]AttrT, 0, len(attrMap))
	for key, val := range attrMap {
		aa = append(aa, AttrT{n: val, s: key})
	}
	sort.Slice(aa, func(a, b int) bool {
		if aa[a].n != aa[b].n {
			return aa[a].n > aa[b].n
		}
		return aa[a].s < aa[b].s
	})
	for _, a := range aa {
		Printf("%d\t%s\n", a.n, a.s)
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
	Printf("Content-type: text/plain; charset=utf-8\n\nError: %v\n", err)
}

func (rc MyReadCloser) Close() error {
	return nil
}

func (rc MyReadCloser) Read(p []byte) (n int, err error) {
	return rc.r.Read(p)
}

func Print(a ...interface{}) {
	_, err := fmt.Print(a...)
	if err != nil {
		chQuit <- true
	}
}

func Println(a ...interface{}) {
	_, err := fmt.Println(a...)
	if err != nil {
		chQuit <- true
	}
}

func Printf(format string, a ...interface{}) {
	_, err := fmt.Printf(format, a...)
	if err != nil {
		chQuit <- true
	}
}
