package main

// TODO: worden alle verbindingen met de database gesloten?
// TODO: worden alle channels gesloten?
// TODO: waitgroups voor alle goroutines
// TODO: 1 globale context en cancel

import (
	ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/url"
	"os"
	"os/signal"
	"regexp"
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
	Fields   []string `json:"fields"`
	Sentid   string   `json:"sentid"`
	Sentence string   `json:"sentence"`
	Ids      []int    `json:"ids"`
	Edges    []*Edge  `json:"edges"`
	Arch     string   `json:"arch"`
}

type EdgeIntern struct {
	label string
	start string
	end   string
}

type Edge struct {
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

var (
	corpora = map[string][2]string{
		"alpinotreebank": [2]string{"alpinotreebank", "/net/corpora/paqu/cdb.dact"},
		"cgn":            [2]string{"cgn", "/net/corpora/paqu/cgnmeta.dact"},
		"eindhoven":      [2]string{"eindhoven", "/net/corpora/paqu/eindhoven.dact"},
		"lassyklein":     [2]string{"lassysmall", "/net/corpora/paqu/lassysmallmeta.dact"},
		"newspapers":     [2]string{"lassynewspapers", ""},
	}

	login string

	// reLimits   = regexp.MustCompile(`((?i)\s+(limit|skip|order)\s+(\d+|all))+\s*$`)
	reQuote    = regexp.MustCompile(`\\.`)
	reComment1 = regexp.MustCompile(`(?s:/\*.*?\*/)`)
	reComment2 = regexp.MustCompile(`--.*`)

	chFinished = make(chan bool)
	chOut      = make(chan string)
	chQuit     = make(chan bool)
	chQuitOpen = true
	muQuit     sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
)

func main() {

	go func() {
		for {
			s, ok := <-chOut
			if !ok {
				close(chFinished)
				return
			}
			_, err := fmt.Print(s)
			if err != nil {
				cancel()
				closeQuit()
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
		<-chFinished
	}()

	corpus, dbname, arch, query, err := parseRequest()
	if err != nil {
		output(fmt.Sprintf("window.parent._fn.error(%q);\n", err.Error()))
		return
	}

	output(fmt.Sprintf("window.parent._fn.db(%q);\n", dbname))

	if strings.HasPrefix(os.Getenv("CONTEXT_DOCUMENT_ROOT"), "/home/peter") {
		login = "port=9333 user=peter dbname=peter sslmode=disable"
	} else {
		login = "user=guest password=guest port=19033 dbname=p209327 sslmode=disable"
		if h, _ := os.Hostname(); !strings.HasPrefix(h, "haytabo") {
			login += " host=haytabo.let.rug.nl"
		}
	}

	go func() {
		chSignal := make(chan os.Signal, 1)
		signal.Notify(chSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		<-chSignal
		closeQuit()
	}()

	start := time.Now()

	go func() {
		for {
			time.Sleep(20 * time.Second)
			output(fmt.Sprintf("window.parent._fn.time('%v');\n", time.Since(start)))
		}
	}()

	err = run(corpus, arch, query)
	if err != nil {
		output(fmt.Sprintf("window.parent._fn.error(%q);\n", err.Error()))
		return
	}

	output(fmt.Sprintf("window.parent._fn.time('%v');\n", time.Since(start)))
	time.Sleep(time.Second)
}

func parseRequest() (corpus string, dbname string, arch string, query string, err error) {
	var req *http.Request
	req, err = cgi.Request()
	if err != nil {
		return
	}

	corpus = req.FormValue("corpus")
	dbname = corpora[corpus][0]
	arch = corpora[corpus][1]
	if dbname == "" {
		err = fmt.Errorf("Invalid or missing corpus %q", corpus)
		return
	}

	query = strings.TrimSpace(req.FormValue("query"))
	if query == "" {
		err = fmt.Errorf("Missing query")
		return
	}

	return
}

func run(corpus, arch, query string) error {

	safequery, err := safeQuery(query, 100, 0)
	if err != nil {
		return err
	}

	chHeader := make(chan []*Header)
	chLine := make(chan *Line)
	chErr := make(chan error) // TODO: close(chErr)

	go doQuery(corpus, arch, safequery, chHeader, chLine, chErr)

	// do Header
	var header []*Header
	var ok bool
	select {
	case err := <-chErr:
		return err
	case header, ok = <-chHeader:
		if !ok {
			return fmt.Errorf("channel chHeader closed")
		}
	}
	b, err := json.Marshal(header)
	if err != nil {
		return err
	}
	output(fmt.Sprintln("window.parent._fn.header(", string(b), ");"))

LOOP:
	for {
		select {
		case <-chQuit:
			return fmt.Errorf("Quit")
		case err := <-chErr:
			return err
		case line, ok := <-chLine:
			if !ok {
				break LOOP
			}
			b, err := json.Marshal(line)
			if err != nil {
				return err
			}
			output(fmt.Sprintln("r(", string(b), ");"))
		}
	}

	return nil
}

func safeQuery(query string, limit, offset int) (string, error) {

	// TODO: limit en offset gebruiken

	// TODO: is dit nog nodig?
	// https://github.com/bitnine-oss/agensgraph/issues/496
	query = strings.Replace(query, ":'", ": '", -1)

	// verwijder alle separators
	query = strings.TrimSpace(strings.Replace(query, ";", " ", -1))

	// TODO: dit gaat niet goed als comment1 genest is in comment2
	query = reComment1.ReplaceAllLiteralString(query, "")
	query = reComment2.ReplaceAllLiteralString(query, "")
	query = strings.TrimSpace(query)

	qu := strings.ToUpper(query)
	if !(strings.HasPrefix(qu, "MATCH") || strings.HasPrefix(qu, "SELECT")) {
		return "", fmt.Errorf("Query must start with MATCH or SELECT")
	}

	// limieten worden in rows.Next() gedaan
	return query, nil

	/*

		// verwijder eventuele buitenste limieten
		// TODO: ook interne limieten vóór UNION, INTERSECT, EXCEPT ??
		query = reLimits.ReplaceAllString(query, "")

		// TODO: interne limieten vóór UNION, INTERSECT, EXCEPT toevoegen ??
		qu := strings.ToUpper(query)
		if strings.HasPrefix(qu, "MATCH") {
			return fmt.Sprintf("%s\nSKIP %d LIMIT %d", query, offset, limit), nil
		}
		if strings.HasPrefix(qu, "SELECT") {
			return fmt.Sprintf("%s\nLIMIT %d OFFSET %d", query, limit, offset), nil
		}
		return "", fmt.Errorf("Query must start with MATCH or SELECT")

	*/

}

func doQuery(corpus, arch, safequery string, chHeader chan []*Header, chLine chan *Line, chErr chan error) {
	var chRow chan []interface{}
	dbOpen := false
	chHeaderOpen := true
	chRowOpen := false
	var db *sql.DB
	defer func() {
		if chHeaderOpen {
			close(chHeader)
		}
		if chRowOpen {
			close(chRow)
		}
		if dbOpen {
			go db.Close()
			time.Sleep(time.Second)
		}
	}()

	var err error
	db, err = sql.Open("postgres", login)
	if err != nil {
		chErr <- err
		return
	}
	dbOpen = true
	err = db.Ping()
	if err != nil {
		chErr <- err
		return
	}

	_, err = db.Exec("set graph_path='" + corpus + "'")
	if err != nil {
		chErr <- err
		return
	}

	ctx, cancel = context.WithCancel(context.Background())
	chDone := make(chan bool)
	defer close(chDone)
	go func() {
		select {
		case <-chDone:
			return
		case <-chQuit:
			cancel()
			return
		}
	}()

	rows, err := db.QueryContext(ctx, safequery)
	if err != nil {
		chErr <- err
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
	go doResults(corpus, arch, headers, chRow, chLine, chErr)

	count := 0
	for rows.Next() {
		select {
		case <-chQuit:
			cancel()
			rows.Close()
		default:
		}
		scans := make([]interface{}, len(ctypes))
		for i := range ctypes {
			scans[i] = new([]uint8)
		}
		err := rows.Scan(scans...)
		if err != nil {
			chErr <- err
			return
		}
		chRow <- scans
		count++
		if count == MAXROWS {
			go rows.Close()
			time.Sleep(time.Second)
			break
		}
	}

}

func doResults(corpus, arch string, header []*Header, chRow chan []interface{}, chLine chan *Line, chErr chan error) {

	dbOpened := false
	var db *sql.DB

	defer func() {
		close(chLine)
		if dbOpened {
			go db.Close()
			time.Sleep(time.Second)
		}
	}()

	chDone := make(chan bool)
	defer close(chDone)
	go func() {
		select {
		case <-chDone:
			return
		case <-chQuit:
			cancel()
			return
		}
	}()

	for {
		select {
		case <-chQuit:
			cancel()
			return
		case scans, ok := <-chRow:
			if !ok {
				return
			}

			idmap := make(map[int]bool)
			// endmap := make(map[int]bool)

			edges := make(map[string]*EdgeIntern)
			nodes := make(map[string]int)

			line := &Line{
				Fields: make([]string, len(scans)),
				Edges:  make([]*Edge, 0),
			}
			for i, v := range scans {
				val := *(v.(*[]uint8))
				for {
					var p ag.BasicPath
					var v ag.BasicVertex
					var e ag.BasicEdge

					if p.Scan(val) == nil {
						for _, v := range p.Vertices {
							if sentid, ok := v.Properties["sentid"]; ok {
								line.Sentid = fmt.Sprint(sentid)
							}
							if id, ok := v.Properties["id"]; ok {
								if iid, err := strconv.Atoi(fmt.Sprint(id)); err == nil {
									idmap[iid] = true
									if v.Id.Valid {
										nodes[v.Id.String()] = iid
									}
								}
							}
						}
						for _, e := range p.Edges {
							if e.Id.Valid && e.Start.Valid && e.End.Valid {
								edge := EdgeIntern{
									label: e.Label,
									start: e.Start.String(),
									end:   e.End.String(),
								}
								edges[e.Id.String()] = &edge
							}
						}
						line.Fields[i] = "PATH"
						break
					}

					if v.Scan(val) == nil {
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
						line.Fields[i] = "VERTEX"
						break
					}

					if e.Scan(val) == nil {
						if e.Id.Valid && e.Start.Valid && e.End.Valid {
							edge := EdgeIntern{
								label: e.Label,
								start: e.Start.String(),
								end:   e.End.String(),
							}
							edges[e.Id.String()] = &edge
						}
						line.Fields[i] = "EDGE"
						break
					}

					if header[i].Name == "sentid" {
						if line.Sentid == "" {
							line.Sentid = unescape(string(val))
						}
					} else if header[i].Name == "id" {
						if id, err := strconv.Atoi(string(val)); err == nil {
							idmap[id] = true
						}
					}
					line.Fields[i] = unescape(string(val))
					break
				}
			} // range scans

			line.Ids = make([]int, 0, len(idmap))
			for key := range idmap {
				line.Ids = append(line.Ids, key)
			}
			sort.Ints(line.Ids)

			if line.Sentid == "" {
				chLine <- line
				continue
			}

			if corpus == "newspapers" {
				line.Arch = url.QueryEscape(getNewspapers(line.Sentid + ".xml"))
			} else {
				line.Arch = url.QueryEscape(arch)
			}

			if !dbOpened {
				var err error
				db, err = sql.Open("postgres", login)
				if err != nil {
					chErr <- err
					return
				}
				dbOpened = true
				err = db.Ping()
				if err != nil {
					chErr <- err
					return
				}
				_, err = db.Exec("set graph_path='" + corpus + "'")
				if err != nil {
					chErr <- err
					return
				}
			}

			// TODO: sanitize sentid
			rows, err := db.QueryContext(ctx, "match (s:sentence{sentid: '"+line.Sentid+"'}) return s.tokens")
			if err != nil {
				chErr <- err
				return
			}
			for rows.Next() {
				select {
				case <-chQuit:
					cancel()
				default:
				}
				var s string
				err := rows.Scan(&s)
				if err != nil {
					rows.Close()
					chErr <- err
					return
				}
				line.Sentence = unescape(s)
			}

			tokens := strings.Fields(line.Sentence)
			endmap := make(map[int]bool)
			for id := range idmap {
				rows, err := db.QueryContext(ctx, fmt.Sprintf("match (:nw{sentid: '%s', id: %d})-[:rel*0..]->(w:word) return w.end", line.Sentid, id))
				if err != nil {
					chErr <- fmt.Errorf("%v: end match", err)
					return
				}
				for rows.Next() {
					select {
					case <-chQuit:
						cancel()
					default:
					}
					var end int
					err := rows.Scan(&end)
					if err != nil {
						rows.Close()
						chErr <- err
						return
					}
					endmap[end] = true
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

			for _, edge := range edges {
				start, ok1 := nodes[edge.start]
				end, ok2 := nodes[edge.end]
				if ok1 && ok2 {
					line.Edges = append(line.Edges, &Edge{
						Label: edge.label,
						Start: start,
						End:   end,
					})
				}
			}

			chLine <- line

		}
	}
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

func getNewspapers(sentid string) string {
	p1 := 0
	p2 := len(np) - 1
	if np[p2][0] <= sentid {
		return "/net/corpora/LassyLarge/WR-P-P-G/DACT/" + np[p2][1]
	}

	for p2-p1 > 1 {
		p := (p1 + p2) / 2
		if np[p][0] > sentid {
			p2 = p
		} else {
			p1 = p
		}
	}
	return "/net/corpora/LassyLarge/WR-P-P-G/DACT/" + np[p1][1]
}

func output(s string) {
	chOut <- fmt.Sprint(`<script type="text/javascript"><!--
` + s + `
</script>
`)
}

func closeQuit() {
	muQuit.Lock()
	defer muQuit.Unlock()
	if chQuitOpen {
		close(chQuit)
		chQuitOpen = false
		time.Sleep(10 * time.Second)
	}
}
