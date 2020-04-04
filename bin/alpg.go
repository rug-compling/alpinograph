package main

import (
	ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http/cgi"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	MAXROWS = 400
)

type Header struct {
	Name string `json:"name"`
}

type Line struct {
	Fields   []string `json:"fields,omitempty"`
	Sentid   string   `json:"sentid,omitempty"`
	Sentence string   `json:"sentence,omitempty"`
	IDs      string   `json:"ids"`
	Rels     string   `json:"rels"`
	UDs      string   `json:"uds"`
	EUDs     string   `json:"euds"`
	Pairs    string   `json:"pairs"`
}

type Edge struct {
	label string
	value string
	start string
	end   string
}

var (
	db *sql.DB

	// reLimits   = regexp.MustCompile(`((?i)\s+(limit|skip|order)\s+(\d+|all))+\s*$`)
	reQuote   = regexp.MustCompile(`\\.`)
	reComment = regexp.MustCompile(`(?s:/\*.*?\*/)|--.*`)

	chOut      = make(chan string)
	chQuit     = make(chan bool)
	chQuitOpen = true
	muQuit     sync.Mutex

	ctx       context.Context
	cancel    context.CancelFunc
	hasCancel = false
	muCancel  sync.Mutex

	wg sync.WaitGroup
)

func main() {

	wg.Add(1)
	go func() {
		for {
			s, ok := <-chOut
			if !ok { // chOut gesloten: klaar
				wg.Done()
				return
			}
			_, err := fmt.Print(s)
			if err != nil { // stdout gesloten
				doQuit()
			}
		}
	}()

	chOut <- `Content-type: text/html; charset=utf-8

<html>
<script type="text/javascript"><!--
function r(s) {
  window.parent._fn.row(s);
}
window.parent._fn.reset();
</script>
`

	defer func() {
		chOut <- `
<script type="text/javascript"><!--
window.parent._fn.done();
</script>
</html>
`
		close(chOut)
		doQuit()
		wg.Wait()
		// fmt.Println("<script type=\"text/javascript\">\nconsole.log(\"main done\");\n</script>")
	}()

	req, err := cgi.Request()
	if err != nil {
		errout(err)
		return
	}

	corpus := strings.Replace(req.FormValue("corpus"), "'", "", -1)
	if corpus == "" {
		errout(fmt.Errorf("Missing corpus"))
		return
	}

	output(fmt.Sprintf("window.parent._fn.cp(%q);", corpus))

	go func() {
		chSignal := make(chan os.Signal, 1)
		signal.Notify(chSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		select {
		case <-chSignal:
			doQuit()
		case <-chQuit:
		}
	}()

	start := time.Now()

	err = run(corpus, req.FormValue("query"), start)
	if err != nil {
		errout(err)
		return
	}

	since(start)
}

func run(corpus, query string, start time.Time) error {

	safequery, err := safeQuery(query)
	if err != nil {
		return err
	}

	err = openDB(corpus)
	if err != nil {
		return err
	}
	defer db.Close()

	chHeader := make(chan []*Header)
	chLine := make(chan *Line)
	chErr := make(chan error)

	go func() {
		doQuery(corpus, safequery, chHeader, chLine, chErr)
		// log("doQuery done")
	}()

	ticker := time.Tick(10 * time.Second)

	// do Header
	var header []*Header
	var ok bool
LOOP1:
	for {
		select {
		case err := <-chErr:
			return err
		case <-ticker:
			since(start)
		case header, ok = <-chHeader:
			if !ok {
				return fmt.Errorf("channel chHeader closed")
			}
			break LOOP1
		}
	}
	b, err := json.Marshal(header)
	if err != nil {
		return err
	}
	output(fmt.Sprintln("window.parent._fn.header(", string(b), ");"))

LOOP2:
	for {
		select {
		case <-chQuit:
			break LOOP2
		case <-ticker:
			since(start)
		case err := <-chErr:
			return err
		case line, ok := <-chLine:
			if !ok {
				break LOOP2
			}
			b, err := json.Marshal(line)
			if err != nil {
				return err
			}
			output("r(" + string(b) + ");")
		}
	}

	return nil
}

func safeQuery(query string) (string, error) {

	// TODO: is dit nog nodig?
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

func safeString(s string) string {
	return strings.Replace(strings.Replace(s, "'", "", -1), `\`, "", -1)
}

func doQuery(corpus, safequery string, chHeader chan []*Header, chLine chan *Line, chErr chan error) {
	var chRow chan []interface{}
	chHeaderOpen := true
	chRowOpen := false
	defer func() {
		if r := recover(); r != nil {
			chErr <- fmt.Errorf("Recovered in doQuery: %v", r)
		}
		if chHeaderOpen {
			close(chHeader)
		}
		if chRowOpen {
			close(chRow)
		}
	}()

	rows, err := db.QueryContext(ctx, qc(corpus, safequery))
	if err != nil {
		chErr <- wrap(err)
		return
	}

	ctypes, _ := rows.ColumnTypes()
	headers := make([]*Header, len(ctypes))
	for i, ct := range ctypes {
		headers[i] = &Header{
			Name: ct.Name(),
		}
	}
	chHeader <- headers
	close(chHeader)
	chHeaderOpen = false

	chRow = make(chan []interface{})
	chRowOpen = true

	go func() {
		doResults(corpus, headers, chRow, chLine, chErr)
		// log("doResults done")
	}()

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
		if count == MAXROWS {
			// rows.Close() // dit hangt
			break
		}
	}
	if err := rows.Err(); err != nil {
		chErr <- wrap(err)
	}
}

func doResults(corpus string, header []*Header, chRow chan []interface{}, chLine chan *Line, chErr chan error) {

	defer func() {
		if r := recover(); r != nil {
			chErr <- fmt.Errorf("Recovered in doResults: %v\n\n%v", r, string(debug.Stack()))
		}
		close(chLine)
	}()

	for {
		select {
		case <-chQuit:
			return
		case scans, ok := <-chRow:
			if !ok {
				// chRow is gesloten: klaar
				return
			}

			idmap := make(map[int]bool)
			edges := make(map[string]*Edge)
			nodes := make(map[string]int)

			line := &Line{
				Fields: make([]string, len(scans)),
			}
			for i, v := range scans {
				val := *(v.(*[]byte))
				sval := string(val)
				line.Fields[i] = html.EscapeString(unescape(sval))
				// log(sval)
				for {

					var p ag.BasicPath
					var v ag.BasicVertex
					var e ag.BasicEdge

					// paden die met een edge beginnen doen een panic in BasicPath.Scan
					// TODO: https://github.com/bitnine-oss/agensgraph-golang/issues/3
					if strings.HasPrefix(sval, "[sentence") ||
						strings.HasPrefix(sval, "[node") ||
						strings.HasPrefix(sval, "[word") ||
						strings.HasPrefix(sval, "[meta") ||
						strings.HasPrefix(sval, "[doc") {
						if p.Scan(val) == nil {
							line.Fields[i] = format(line.Fields[i])
							//n := len(p.Vertices) - 1
							for _, v := range p.Vertices {
								if sentid, ok := v.Properties["sentid"]; ok {
									line.Sentid = fmt.Sprint(sentid)
								}
								if id, ok := v.Properties["id"]; ok {
									if iid, err := strconv.Atoi(fmt.Sprint(id)); err == nil {
										//if i == 0 || i == n {
										idmap[iid] = true
										//}
										if v.Id.Valid {
											nodes[v.Id.String()] = iid
										}
									}
								}
							}
							for _, e := range p.Edges {
								if e.Id.Valid && e.Start.Valid && e.End.Valid {
									edge := Edge{
										label: e.Label,
										start: e.Start.String(),
										end:   e.End.String(),
									}
									if e.Label == "pair" {
										edge.value = unescape(fmt.Sprint(e.Properties["rel"]))
									}
									edges[e.Id.String()] = &edge
								}
							}
							break
						}
					}

					if v.Scan(val) == nil {
						line.Fields[i] = format(line.Fields[i])
						if sentid, ok := v.Properties["sentid"]; ok {
							line.Sentid = fmt.Sprint(sentid)
						}
						if id, ok := v.Properties["id"]; ok {
							if iid, err := strconv.Atoi(string(fmt.Sprint(id))); err == nil {
								idmap[iid] = true
								if v.Id.Valid {
									nodes[v.Id.String()] = iid
								}
							}
						}
						break
					}

					if e.Scan(val) == nil {
						line.Fields[i] = format(line.Fields[i])
						if e.Id.Valid && e.Start.Valid && e.End.Valid {
							edge := Edge{
								label: e.Label,
								start: e.Start.String(),
								end:   e.End.String(),
							}
							edges[e.Id.String()] = &edge
						}
						break
					}

					if header[i].Name == "sentid" {
						if line.Sentid == "" {
							line.Sentid = unescape(sval)
						}
					} else if header[i].Name == "id" {
						if id, err := strconv.Atoi(sval); err == nil {
							idmap[id] = true
						}
					}
					break
				} // for
			} // range scans

			ints := make([]int, 0, len(idmap))
			for key := range idmap {
				ints = append(ints, key)
			}
			sort.Ints(ints)
			intss := make([]string, 0, len(ints))
			for _, i := range ints {
				intss = append(intss, fmt.Sprint(i))
			}
			line.IDs = strings.Join(intss, ",")

			if line.Sentid == "" {
				chLine <- line
				continue
			}

			rows, err := db.QueryContext(ctx, qc(corpus, "match (s:sentence{sentid: '"+safeString(line.Sentid)+"'}) return s.tokens"))
			if err != nil {
				chErr <- wrap(err)
				return
			}
			for rows.Next() {
				var s string
				err := rows.Scan(&s)
				if err != nil {
					// rows.Close()
					chErr <- wrap(err)
					return
				}
				line.Sentence = unescape(s)
				select {
				case <-chQuit:
					// rows.Close()
					return
				default:
				}
			}

			tokens := strings.Fields(line.Sentence)
			endmap := make(map[int]bool)
			for id := range idmap {
				rows, err := db.QueryContext(
					ctx,
					qc(corpus, fmt.Sprintf("match (:nw{sentid: '%s', id: %d})-[:rel*0..]->(w:word) return w.end", line.Sentid, id)))
				if err != nil {
					chErr <- wrap(err)
					return
				}
				for rows.Next() {
					var end int
					err := rows.Scan(&end)
					if err != nil {
						// rows.Close()
						chErr <- wrap(err)
						return
					}
					endmap[end] = true
					select {
					case <-chQuit:
						// rows.Close()
						return
					default:
					}
				}
				if err := rows.Err(); err != nil {
					chErr <- wrap(err)
				}
			}
			inMark := false
			for i := 0; i < len(tokens); i++ {
				if endmap[i+1] {
					if !inMark {
						inMark = true
						tokens[i] = `<span class="mark">` + tokens[i]
					}
				} else {
					if inMark {
						inMark = false
						tokens[i-1] = tokens[i-1] + `</span>`
					}
				}
			}
			if inMark {
				tokens[len(tokens)-1] = tokens[len(tokens)-1] + `</span>`
			}

			line.Sentence = strings.Join(tokens, " ")

			rels := make([]string, 0)
			uds := make([]string, 0)
			euds := make([]string, 0)
			pairs := make([]string, 0)
			for _, edge := range edges {
				start, ok1 := nodes[edge.start]
				end, ok2 := nodes[edge.end]
				if ok1 && ok2 {
					link := fmt.Sprintf("%d-%d", start, end)
					if edge.label == "rel" {
						rels = append(rels, link)
					} else if edge.label == "ud" {
						uds = append(uds, link)
					} else if edge.label == "eud" {
						euds = append(euds, link)
					} else if edge.label == "pair" {
						pairs = append(pairs, link+"-"+url.PathEscape(edge.value))
					}
				}
			}
			line.Rels = strings.Join(rels, ",")
			line.UDs = strings.Join(uds, ",")
			line.EUDs = strings.Join(euds, ",")
			line.Pairs = strings.Join(pairs, ",")

			chLine <- line

		} // select

	} // for
}

func toINT(v interface{}) int {
	switch t := v.(type) {
	case string:
		i, err := strconv.Atoi(unescape(t))
		if err == nil {
			return i
		}
		return -999
	case int:
		return t
	case float64:
		return int(t)
	}
	return -999
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

func doQuit() {
	muQuit.Lock()
	if chQuitOpen {
		close(chQuit)
		chQuitOpen = false
	}
	muQuit.Unlock()
	doCancel()
}

func doCancel() {
	muCancel.Lock()
	if hasCancel {
		cancel()
		hasCancel = false
	}
	muCancel.Unlock()
}

func since(start time.Time) {
	var s string
	dur := time.Since(start)
	if dur < time.Second {
		s = fmt.Sprintf("%.3fms", float64(dur)/float64(time.Millisecond))
	} else if dur < 10*time.Second {
		s = fmt.Sprintf("%.1fs", float64(dur)/float64(time.Second))
	} else if dur < time.Hour {
		s = fmt.Sprintf("%d:%02d", dur/time.Minute, (dur%time.Minute)/time.Second)
	} else {
		s = fmt.Sprintf("%d:%02d:%02d", dur/time.Hour, (dur%time.Hour)/time.Minute, (dur%time.Minute)/time.Second)
	}
	output(fmt.Sprintf("window.parent._fn.time('Tijd: %s');", s))
}

func openDB(corpus string) error {

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

func qc(corpus, query string) string {
	return fmt.Sprintf("set graph_path='%s';\n%s", corpus, query)
}

func output(s string) {
	chOut <- fmt.Sprint(`<script type="text/javascript">
` + s + `
</script>
`)
}

func log(s string) {
	output(fmt.Sprintf("console.log(%q);", s))
}

func errout(err error) {
	output(fmt.Sprintf("window.parent._fn.error(%q);", err.Error()))
}

func wrap(err error) error {
	_, filename, lineno, ok := runtime.Caller(1)
	if ok {
		return fmt.Errorf("%v:%v: %v", filename, lineno, err)
	}
	return err
}

func format(s string) string {
	if s[0] == '[' {
		s = s[1:]
	} else {
		s = s + "]"
	}
	s = strings.Replace(s, "]{},", "]\n<table class=\"inner\">\n<tr><td></td></tr>\n</table>\n</div>\n<div class=\"inner\">", -1)
	s = strings.Replace(s, "]{}]", "]\n</div>\n", -1)
	s = strings.Replace(s, "]{&#34;", "]\n<table class=\"inner\">\n<tr><td>", -1)
	s = strings.Replace(s, ", &#34;", "</td></tr>\n<tr><td>", -1)
	s = strings.Replace(s, "&#34;: ", "</td><td>", -1)
	s = strings.Replace(s, "},", "</td></tr>\n</table>\n</div>\n<div class=\"inner\">\n", -1)
	s = strings.Replace(s, "}]", "</td></tr>\n</table>\n</div>\n", 1)
	s = "<div class=\"inner\">" + s
	return s
}
