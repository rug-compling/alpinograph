package main

import (
	ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
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
	MAXSENT  = 40
	MAXROWS  = 400
	MAXWORDS = 20000
)

type gbool struct {
	b bool
	l sync.RWMutex
}

type gint struct {
	i int
	l sync.RWMutex
}

type Header struct {
	Name string `json:"name"`
}

type Line struct {
	LineNo   int      `json:"lineno,emitempty"`
	Fields   []string `json:"fields,omitempty"`
	Sentence string   `json:"sentence,omitempty"`
	Args     string   `json:"args,omitempty"`
}

type Edge struct {
	label  string
	value  string
	start  string
	end    string
	eStart string
	eEnd   string
}

type Thing struct {
	id, from, to string
	label        string
	props        string
}

type IntStr struct {
	i int
	s string
}

var (
	db *sql.DB

	// reLimits   = regexp.MustCompile(`((?i)\s+(limit|skip|order)\s+(\d+|all))+\s*$`)
	reQuote   = regexp.MustCompile(`\\.`)
	reComment = regexp.MustCompile(`(?s:/\*.*?\*/)|--.*`)

	chOut    = make(chan string)
	chQuit   = make(chan bool)
	onceQuit sync.Once

	tooManyWords = &gbool{}
	wordCount    = make(map[string]int)
	lemmaCount   = make(map[string]int)
	muWords      sync.Mutex

	ctx       context.Context
	cancel    context.CancelFunc
	hasCancel = gbool{}

	offset     = 0
	pagesize   = &gint{i: MAXROWS}
	paging     = false
	oncePaging sync.Once

	wg    sync.WaitGroup
	start time.Time
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
function clw() {
  window.parent._fn.clearwords();
}
function cll() {
  window.parent._fn.clearlemmas();
}
function ww(i, n, s) {
  window.parent._fn.setwords(i, n, s);
}
function wl(i, n, s) {
  window.parent._fn.setlemmas(i, n, s);
}
function sw(i, n, s) {
  window.parent._fn.skipwords();
}
function sl(i, n, s) {
  window.parent._fn.skiplemmas();
}
function mw(s, done) {
  window.parent._fn.setwordsmsg(s, done);
}
function ml(s, done) {
  window.parent._fn.setlemmasmsg(s, done);
}
</script>
`
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

	paging = req.FormValue("paging") != ""
	if paging {
		offset, _ = strconv.Atoi(req.FormValue("offset"))
	}

	defer func() {
		chOut <- fmt.Sprintf(`
<script type="text/javascript"><!--
window.parent._fn.done(%v);
</script>
</html>
`, paging)
		close(chOut)
		doQuit()
		wg.Wait()
		// fmt.Println("<script type=\"text/javascript\">\nconsole.log(\"main done\");\n</script>")
	}()

	go func() {
		chSignal := make(chan os.Signal, 1)
		signal.Notify(chSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		select {
		case <-chSignal:
			doQuit()
		case <-chQuit:
		}
	}()

	start = time.Now()

	err = run(corpus, req.FormValue("query"), start)
	if err != nil {
		errout(err)
		return
	}
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
			if !paging {
				doTables(false)
			}
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
	if !paging {
		doTables(true)
	}
	return nil
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

	rows, err := db.QueryContext(ctx, qsafe(corpus, safequery))
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
		count++
		if count <= offset {
			continue
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
		if paging && count-offset > pagesize.get() {
			doPaging(true)
			break
		}
		if count-offset == MAXROWS {
			doPaging(true)
			muWords.Lock()
			n := len(wordCount)
			muWords.Unlock()
			if n == 0 {
				// rows.Close() // dit hangt
				break
			}
		}
		if count-offset > MAXROWS {
			if tooManyWords.get() {
				break
			}
		}
	}
	doPaging(false)
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

	count := 0
	scSeen := make(map[string]bool)
	wlSeen := make(map[string]bool)
RESULTS:
	for {
		select {
		case <-chQuit:
			return
		case scans, ok := <-chRow:
			if !ok {
				// chRow is gesloten: klaar
				return
			}
			count++

			if count == pagesize.get()+1 {
				doPaging(true)
			}

			sc := fmt.Sprint(scans...)
			if scSeen[sc] {
				continue RESULTS
			}
			scSeen[sc] = true

			var sentid string

			idmap := make(map[int]bool)
			edges := make(map[string]*Edge)
			nodes := make(map[string]int)

			line := &Line{
				LineNo: count,
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
							verticeThings := make([]*Thing, 0)
							edgeThings := make([]*Thing, 0)
							//n := len(p.Vertices) - 1
							for _, v := range p.Vertices {
								verticeThings = append(verticeThings, getVertex(&v))
								if sid, ok := v.Properties["sentid"]; ok {
									sentid = fmt.Sprint(sid)
								}
								if v.Label == "sentence" {
									nodes[v.Id.String()] = -1
								} else if id, ok := v.Properties["id"]; ok {
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
								edgeThings = append(edgeThings, getEdge(&e))
								if e.Id.Valid && e.Start.Valid && e.End.Valid {
									var rel, start, end string
									if e.Label != "next" {
										rel = unescape(fmt.Sprint(e.Properties["rel"]))
									}
									if e.Label == "eud" {
										if s, ok := e.Properties["from"]; ok {
											start = unescape(fmt.Sprint(s))
										}
										if s, ok := e.Properties["to"]; ok {
											end = unescape(fmt.Sprint(s))
										}
									}
									edges[e.Id.String()] = &Edge{
										label:  e.Label,
										start:  e.Start.String(),
										end:    e.End.String(),
										value:  rel,
										eStart: start,
										eEnd:   end,
									}
								}
							}
							line.Fields[i] = formatThings(verticeThings, edgeThings)
							if line.Fields[i] == "" {
								line.Fields[i] = html.EscapeString(unescape(sval))
							}
							break
						}
					}

					if v.Scan(val) == nil {
						if sid, ok := v.Properties["sentid"]; ok {
							sentid = fmt.Sprint(sid)
						}
						if v.Label == "sentence" {
							nodes[v.Id.String()] = -1
						} else if id, ok := v.Properties["id"]; ok {
							if iid, err := strconv.Atoi(string(fmt.Sprint(id))); err == nil {
								idmap[iid] = true
								if v.Id.Valid {
									nodes[v.Id.String()] = iid
								}
							}
						}
						line.Fields[i] = formatThings([]*Thing{getVertex(&v)}, []*Thing{})
						if line.Fields[i] == "" {
							line.Fields[i] = html.EscapeString(unescape(sval))
						}
						break
					}

					if e.Scan(val) == nil {
						if e.Id.Valid && e.Start.Valid && e.End.Valid {
							var rel, start, end string
							if e.Label != "next" {
								rel = unescape(fmt.Sprint(e.Properties["rel"]))
							}
							if e.Label == "eud" {
								if s, ok := e.Properties["from"]; ok {
									start = unescape(fmt.Sprint(s))
								}
								if s, ok := e.Properties["to"]; ok {
									end = unescape(fmt.Sprint(s))
								}
							}
							edges[e.Id.String()] = &Edge{
								label:  e.Label,
								start:  e.Start.String(),
								end:    e.End.String(),
								value:  rel,
								eStart: start,
								eEnd:   end,
							}
						}
						line.Fields[i] = formatThings([]*Thing{getEdge(&e)}, []*Thing{})
						if line.Fields[i] == "" {
							line.Fields[i] = html.EscapeString(unescape(sval))
						}
						break
					}

					if header[i].Name == "sentid" {
						if sentid == "" {
							sentid = unescape(sval)
						}
					} else if header[i].Name == "id" {
						if id, err := strconv.Atoi(sval); err == nil {
							idmap[id] = true
						}
					}
					break
				} // for
			} // range scans

			if sentid == "" {
				if count <= MAXROWS {
					chLine <- line
				}
				continue
			}
			if count == 1 {
				pagesize.set(MAXSENT)
			}

			ints := make([]int, 0, len(idmap))
			for key := range idmap {
				ints = append(ints, key)
			}
			sort.Ints(ints)
			intss := make([]string, 0, len(ints))
			for _, i := range ints {
				intss = append(intss, fmt.Sprint(i))
			}
			IDs := strings.Join(intss, ",")

			rows, err := db.QueryContext(ctx, qc(corpus, "match (s:sentence{sentid: '"+safeString(sentid)+"'}) return s.tokens"))
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
			endmap := make(map[int][2]string)
			// TODO: dit kan in 1 keer, zie api.go
			for id := range idmap {
				rows, err := db.QueryContext(
					ctx,
					qc(corpus, fmt.Sprintf("match (:nw{sentid: '%s', id: %d})-[:rel*0..]->(w:word) return w.end, w.word, w.lemma", sentid, id)))
				if err != nil {
					chErr <- wrap(err)
					return
				}
				for rows.Next() {
					var end int
					var word, lemma string
					err := rows.Scan(&end, &word, &lemma)
					if err != nil {
						// rows.Close()
						chErr <- wrap(err)
						return
					}
					endmap[end] = [2]string{unescape(word), unescape(lemma)}
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
			hasIDs := len(endmap) > 0
			if hasIDs && count == 1 && !paging {
				output("window.parent._fn.wordstart();\n")
			}

			inMark := false
			started := false
			words := make([]string, 0)
			lemmas := make([]string, 0)
			ids := []string{sentid}
			for i := 0; i < len(tokens); i++ {
				if wl, ok := endmap[i+1]; ok {
					if !inMark {
						inMark = true
						tokens[i] = `<span class="mark">` + tokens[i]
						if started {
							words = append(words, "[...]")
							lemmas = append(lemmas, "[...]")
						}
						started = true
					}
					words = append(words, wl[0])
					lemmas = append(lemmas, wl[1])
					ids = append(ids, fmt.Sprint(i))
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

			if hasIDs {
				muWords.Lock()
				idss := strings.Join(ids, " ")
				if !wlSeen[idss] {
					wlSeen[idss] = true
					ww := strings.Join(words, " ")
					ll := strings.Join(lemmas, " ")
					wordCount[ww] = wordCount[ww] + 1
					lemmaCount[ll] = lemmaCount[ll] + 1
				}
				muWords.Unlock()
			}

			if count > MAXSENT {
				continue
			}

			line.Sentence = strings.Join(tokens, " ")

			rr := make([]string, 0)
			for _, edge := range edges {
				start, ok1 := nodes[edge.start]
				end, ok2 := nodes[edge.end]
				var eStart, eEnd string
				if ok1 && ok2 {
					p := ""
					if edge.label == "rel" {
						p = "r"
					} else if edge.label == "ud" {
						p = "u"
					} else if edge.label == "eud" {
						p = "e"
						if edge.eStart != "" {
							eStart = "!" + edge.eStart
						}
						if edge.eEnd != "" {
							eEnd = "!" + edge.eEnd
						}
					} else if edge.label == "pair" {
						p = "p"
					} else if edge.label == "next" {
						p = "n"
					}
					rr = append(rr, fmt.Sprintf("%s_%d%s_%d%s_%s", p, start, eStart, end, eEnd, url.PathEscape(edge.value)))
				}
			}
			line.Args = fmt.Sprintf("c=%s&s=%s&i=%s&e=%s", corpus, url.PathEscape(sentid), IDs, strings.Join(rr, ","))

			chLine <- line

		} // select

	} // for
}

func getVertex(v *ag.BasicVertex) *Thing {
	return &Thing{
		id:    v.Id.String(),
		label: v.Label,
		props: formatProperties(v.Properties),
	}
}

func getEdge(e *ag.BasicEdge) *Thing {
	return &Thing{
		id:    e.Id.String(),
		from:  e.Start.String(),
		to:    e.End.String(),
		label: e.Label,
		props: formatProperties(e.Properties),
	}
}

func formatThings(vv []*Thing, ee []*Thing) string {
	vn := len(vv)
	en := len(ee)
	if vn != en+1 {
		return ""
	}

	var buf bytes.Buffer

	vorige := "NONE"
	for i := 0; i < vn+en; i++ {
		var item *Thing
		if i%2 == 0 {
			item = vv[i/2]
		} else {
			item = ee[i/2]
		}
		e1 := "(:"
		e2 := ")"
		if item.from == vorige {
			e1 = "-[:"
			e2 = "]-&gt;"
		} else if item.to == vorige {
			e1 = "&lt;-[:"
			e2 = "]-"
		} else if item.from != "" || item.to != "" {
			e1 = "?-[:"
			e2 = "]-?"
		}
		fmt.Fprintf(&buf, "<div class=\"inner\"><code>%s%s%s</code>%s</div>\n", e1, item.label, e2, item.props)
		vorige = item.id
	}

	return buf.String()
}

func formatProperties(ii map[string]interface{}) string {
	keys := make([]string, 0)
	for key := range ii {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a][0] == '_' {
			if keys[b][0] != '_' {
				return false
			}
		} else {
			if keys[b][0] == '_' {
				return true
			}
		}
		return strings.ToLower(keys[a]) < strings.ToLower(keys[b])
	})
	var buf bytes.Buffer
	buf.WriteString("<table class=\"inner\">\n")
	if len(keys) > 0 {
		for _, key := range keys {
			fmt.Fprintf(&buf,
				"<tr><td>%s</td><td class=\"%T\">%s</td></tr>\n",
				html.EscapeString(key),
				ii[key],
				html.EscapeString(fmt.Sprint(ii[key])))
		}
	} else {
		fmt.Fprintln(&buf, "<tr><td></td></tr>")
	}
	buf.WriteString("</table>\n")
	return buf.String()
}

func doTables(final bool) {

	for i := 0; i < 2; i++ {
		var count map[string]int
		var total, subTotal, vars, subVars int

		muWords.Lock()
		if i == 0 {
			count = wordCount
		} else {
			count = lemmaCount
		}
		n := len(count)
		if n == 0 {
			muWords.Unlock()
			continue
		}
		items := make([]IntStr, 0, n)
		for key, value := range count {
			items = append(items, IntStr{i: value, s: key})
		}
		muWords.Unlock()

		sort.Slice(items, func(a, b int) bool {
			if items[a].i != items[b].i {
				return items[a].i > items[b].i
			}
			return items[a].s < items[b].s
		})

		var s string
		if i == 0 {
			s = "w"
		} else {
			s = "l"
		}
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "cl%s();\n", s)
		lim1 := len(items)
		lim2 := 0
		if lim1 > MAXROWS {
			lim2 = lim1 - MAXROWS/2
			lim1 = MAXROWS / 2
		}
		for j, item := range items {
			total += item.i
			vars++
			if j == lim1 {
				fmt.Fprintf(&buf, "s%s();\n", s)
			} else if j < lim1 || j >= lim2 {
				fmt.Fprintf(&buf, "w%s(%d, %q, %q);\n", s, j+1, numFormat(item.i), item.s)
				subTotal += item.i
				subVars++
			}
		}

		var status string
		if total > MAXWORDS {
			status = "<br>limiet bereikt"
			tooManyWords.set(true)
		} else if final {
			status = ""
		} else {
			status = "<br>bezig..."
		}
		fmt.Fprintf(&buf, "m%s('varianten: %s", s, numFormat(vars))
		if vars > subVars {
			fmt.Fprintf(&buf, " (%s getoond)", numFormat(subVars))
		}
		fmt.Fprintf(&buf, "<br>totaal: %s", numFormat(total))
		if total > subTotal {
			fmt.Fprintf(&buf, " (%s getoond)", numFormat(subTotal))
		}
		fmt.Fprintf(&buf, "%s', %v);\n", status, total > MAXWORDS || final)
		output(buf.String())
	}
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
	onceQuit.Do(func() {
		close(chQuit)
	})
	doCancel()
}

func doCancel() {
	if hasCancel.get() {
		cancel()
		hasCancel.set(false)
	}
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
	output(fmt.Sprintf("window.parent._fn.time('Tijd: %s', %v);", s, paging))
}

func openDB(corpus string) error {

	b, err := ioutil.ReadFile("login")
	if err != nil {
		return err
	}
	login := strings.TrimSpace(string(b))

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
	hasCancel.set(true)

	return nil
}

func qc(corpus, query string) string {
	return fmt.Sprintf("set graph_path='%s';\n%s", corpus, query)
}

func qsafe(corpus, query string) string {
	return fmt.Sprintf("begin; set graph_path='%s';\n%s;\nrollback", corpus, query)
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

func numFormat(i int) string {
	s1 := fmt.Sprint(i)
	s2 := ""
	for n := len(s1); n > 3; n = len(s1) {
		// U+202F = NARROW NO-BREAK SPACE
		//s2 = "&#8239;" + s1[n-3:n] + s2
		s2 = "." + s1[n-3:n] + s2
		s1 = s1[0 : n-3]
	}
	return s1 + s2
}

func doPaging(more bool) {
	oncePaging.Do(func() {
		since(start)
		p := pagesize.get()
		output(fmt.Sprintf("window.parent._fn.setPaging(%d, %d, %v);", offset-p, offset+p, more))
	})
}

func (g *gint) set(i int) {
	g.l.Lock()
	g.i = i
	g.l.Unlock()
}

func (g *gint) get() int {
	g.l.RLock()
	i := g.i
	g.l.RUnlock()
	return i
}

func (g *gbool) set(b bool) {
	g.l.Lock()
	g.b = b
	g.l.Unlock()
}

func (g *gbool) get() bool {
	g.l.RLock()
	b := g.b
	g.l.RUnlock()
	return b
}
