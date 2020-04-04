package main

import (
	ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	"bytes"
	"database/sql"
	"fmt"
	"html"
	"net/http/cgi"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type Edge struct {
	from  int
	to    int
	rel   string
	orito int
}

type Word struct {
	id  int
	end int
}

var (
	reQuote = regexp.MustCompile(`\\.`)
	db      *sql.DB
)

func main() {

	req, err := cgi.Request()
	if x(err) {
		return
	}

	err = req.ParseForm()
	if x(err) {
		return
	}

	corpus := strings.Replace(req.FormValue("c"), "'", "", -1)
	sid := strings.Replace(req.FormValue("s"), "'", "", -1)
	idlist := strings.TrimSpace(req.FormValue("i"))
	rellist := strings.TrimSpace(req.FormValue("r"))
	compact := req.FormValue("style") == "compact"

	if x(openDB()) {
		return
	}

	_, err = db.Exec("set graph_path='" + corpus + "'")
	if x(err) {
		return
	}

	row := db.QueryRow("match (s:sentence{sentid:'" + sid + "'}) return s.tokens")
	var zin string
	if row != nil {
		if x(row.Scan(&zin)) {
			return
		}
		zin = html.EscapeString(unescape(zin))
	}

	meta, ok := makeMeta(corpus, sid)
	if !ok {
		return
	}

	parser, ok := makeParser(corpus, sid)
	if !ok {
		return
	}

	tree, ok := makeTree(corpus, sid, idlist, rellist, compact)
	if !ok {
		return
	}

	cmd := exec.Command("dot", "-Tsvg")
	cmd.Stdin = tree
	b, err := cmd.CombinedOutput()
	if x(err) {
		return
	}
	svg, ok := postProcess(string(b))
	if !ok {
		return
	}

	c, ok := corpora[corpus]
	if !ok {
		c = corpus
	}

	fmt.Printf(`Content-type: text/html; charset=utf-8

<html>
<head>
<title>%s</title>
<link rel="stylesheet" type="text/css" href="../tree.css">
<link rel="stylesheet" type="text/css" href="../tooltip.css" />
<script type="text/javascript" src="../tooltip.js"></script>
</head>
<body>
<em>%s</em>
<p>
corpus: %s<br>
sentence-ID: %s%s
%s
<div class="svg">
%s
</div>
</body>
</html>
`, zin, zin, c, sid, parser, meta, svg)

}

func makeMeta(corpus, sid string) (meta string, ok bool) {

	lines := make([]string, 0)

	rows, err := db.Query("match (m:meta{sentid:'" + sid + "'}) return m.name, m.type, m.value")
	if x(err) {
		return
	}

	for rows.Next() {
		var name, tp, val string
		if x(rows.Scan(&name, &tp, &val)) {
			return
		}
		lines = append(lines,
			fmt.Sprintf(
				"[%s] %s: %s",
				html.EscapeString(unescape(tp)),
				html.EscapeString(unescape(name)),
				html.EscapeString(unescape(val))))
	}
	x(rows.Err())

	if len(lines) == 0 {
		return "", true
	}
	return "<p>\n" + strings.Join(lines, "<br>\n") + "\n</p>\n", true
}

func makeParser(corpus, sid string) (parser string, ok bool) {

	rows, err := db.Query("match (s:sentence{sentid:'" + sid + "'}) return s.cats, s.skips")
	if x(err) {
		return
	}

	var c, s sql.NullInt64
	for rows.Next() {
		if x(rows.Scan(&c, &s)) {
			return
		}
	}
	x(rows.Err())
	if c.Valid {
		parser = fmt.Sprintf("<br>\ncats: %d", c.Int64)
	}
	if s.Valid {
		parser += fmt.Sprintf("<br>\nskips: %d", s.Int64)
	}

	return parser, true
}

func makeTree(corpus, sid, idlist, rellist string, compact bool) (tree *bytes.Buffer, ok bool) {

	idmap := make(map[int]bool)
	if idlist != "" {
		for _, id := range strings.Split(idlist, ",") {
			i, err := strconv.Atoi(id)
			if x(err) {
				return
			}
			idmap[i] = true
		}
	}

	relmap := make(map[string]bool)
	if rellist != "" {
		for _, rel := range strings.Split(rellist, ",") {
			relmap[rel] = true
		}
	}

	rows, err := db.Query("match (n1:node{sentid:'" + sid + "'})-[r:rel]->(n2:nw) return n1, r, n2 order by n1.id, n2.id")
	if x(err) {
		return
	}

	nodes := make([]ag.BasicVertex, 0)
	links := make([]Edge, 0)
	words := make([]Word, 0)

	seen := make(map[int]bool)
	for rows.Next() {
		var n1, r, n2 []byte
		if x(rows.Scan(&n1, &r, &n2)) {
			return
		}

		var v1 ag.BasicVertex
		var e ag.BasicEdge
		var v2 ag.BasicVertex

		var id1, id2 int

		if x(v1.Scan(n1)) {
			return
		}
		if x(e.Scan(r)) {
			return
		}
		if x(v2.Scan(n2)) {
			return
		}
		id1 = toInt(v1.Properties["id"])
		id2 = toInt(v2.Properties["id"])

		if !seen[id1] {
			seen[id1] = true
			nodes = append(nodes, v1)
		}
		if !seen[id2] {
			seen[id2] = true
			nodes = append(nodes, v2)

			if v2.Label == "word" {
				words = append(words, Word{
					id:  toInt(v2.Properties["id"]),
					end: toInt(v2.Properties["end"]),
				})
			}
		}

		link := Edge{
			from: id1,
			to:   id2,
			rel:  toString(e.Properties["rel"]),
		}
		if id, ok := e.Properties["id"]; ok {
			link.orito = toInt(id)
		} else {
			link.orito = link.to
		}
		links = append(links, link)
	}
	if x(rows.Err()) {
		return
	}

	if len(nodes) == 0 {
		fmt.Println("Niet gevonden")
		return
	}

	sort.Slice(words, func(a, b int) bool {
		return words[a].end < words[b].end
	})

	sort.Slice(links, func(a, b int) bool {
		if links[a].from == links[b].from {
			return links[a].orito < links[b].orito
		}
		return links[a].from < links[b].from
	})

	var buf bytes.Buffer

	fmt.Fprint(&buf, `strict graph gr {

    ranksep=".25 equally"
    nodesep=.05
    // ordering=out

    node [shape=box, height=0, width=0, style=filled, fontsize=12, color="#ffc0c0", fontname="Helvetica"];

`)

	for _, node := range nodes {
		if node.Label == "node" {
			tooltip := makeTooltip(node.Properties)
			id := toInt(node.Properties["id"])
			if idmap[id] {
				fmt.Fprintf(&buf, "    n%d [label=%q, style=bold, color=\"#ff0000\", tooltip=%q];\n", id, toString(node.Properties["cat"]), tooltip)
			} else {
				fmt.Fprintf(&buf, "    n%d [label=%q, tooltip=%q];\n", id, toString(node.Properties["cat"]), tooltip)
			}
		}
	}

	fmt.Fprint(&buf, `
    node [fontname="Helvetica-Oblique", color="#c0c0ff"];

`)

	for _, node := range nodes {
		if node.Label == "word" {
			tooltip := makeTooltip(node.Properties)
			id := toInt(node.Properties["id"])
			if idmap[id] {
				fmt.Fprintf(&buf, "    n%d [label=%q, style=bold, color=\"#0000ff\", tooltip=%q];\n", id, toString(node.Properties["word"]), tooltip)
			} else {
				fmt.Fprintf(&buf, "    n%d [label=%q, tooltip=%q];\n", id, toString(node.Properties["word"]), tooltip)
			}
		}
	}

	if !compact {
		fmt.Fprint(&buf, `
    node [fontname="Helvetica", shape=plaintext, style=solid, fontsize=10];

`)

		for _, link := range links {
			if link.rel != "" && link.rel != "--" {
				fmt.Fprintf(&buf, "    n%dn%d [label=%q];\n", link.from, link.to, link.rel)
			}
		}
	}

	fmt.Fprintf(&buf, "\n    {rank=same; edge[style=invis]; n%d", words[0].id)
	for _, w := range words[1:] {
		fmt.Fprintf(&buf, " -- n%d", w.id)
	}

	if compact {
		fmt.Fprintf(&buf, `}

    edge [sametail=true, samehead=true, color="#d3d3d3", fontname="Helvetica", fontsize=10];

`)
	} else {
		fmt.Fprintf(&buf, `}

    edge [sametail=true, samehead=true, color="#d3d3d3"];

`)
	}

	for _, link := range links {
		if relmap[fmt.Sprintf("%d-%d", link.from, link.to)] {
			if link.rel != "" && link.rel != "--" {
				if compact {
					fmt.Fprintf(&buf, "    n%d -- n%d [label=%q, color=\"#000000\"];\n", link.from, link.to, link.rel)
				} else {
					fmt.Fprintf(&buf, "    n%d -- n%dn%d -- n%d [color=\"#000000\"];\n", link.from, link.from, link.to, link.to)
				}
			} else {
				fmt.Fprintf(&buf, "    n%d -- n%d [color=\"#000000\"];\n", link.from, link.to)
			}
		} else {
			if link.rel != "" && link.rel != "--" {
				if compact {
					fmt.Fprintf(&buf, "    n%d -- n%d [label=%q];\n", link.from, link.to, link.rel)
				} else {
					fmt.Fprintf(&buf, "    n%d -- n%dn%d -- n%d;\n", link.from, link.from, link.to, link.to)
				}
			} else {
				fmt.Fprintf(&buf, "    n%d -- n%d;\n", link.from, link.to)
			}
		}
	}

	fmt.Fprint(&buf, "\n}\n")

	return &buf, true
}

func postProcess(svg string) (string, bool) {
	// XML-declaratie en DOCtype overslaan
	if i := strings.Index(svg, "<svg"); i < 0 {
		x(fmt.Errorf("BUG"))
		return "", false
	} else {
		svg = svg[i:]
	}

	lines := make([]string, 0)
	a := ""
	for _, line := range strings.SplitAfter(svg, "\n") {
		// alles wat begint met <title> weghalen
		i := strings.Index(line, "<title")
		if i >= 0 {
			line = line[:i] + "\n"
		}

		// <a xlink> omzetten in tooltip
		i = strings.Index(line, "<a xlink")
		if i >= 0 {
			s := line[i:]
			line = line[:i] + "\n"
			i = strings.Index(s, "\"")
			s = s[i+1:]
			i = strings.LastIndex(s, "\"")
			a = strings.TrimSpace(s[:i])

		}
		if strings.HasPrefix(line, "<text ") && a != "" {
			line = "<text onmouseover=\"tooltip.show('" + html.EscapeString(a) + "')\" onmouseout=\"tooltip.hide()\"" + line[5:]
		}
		if strings.HasPrefix(line, "</a>") {
			line = ""
			a = ""
		}

		lines = append(lines, line)
	}
	return strings.Join(lines, ""), true
}

func makeTooltip(p map[string]interface{}) string {
	keys := make([]string, 0, len(p))
	for key := range p {
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
	buf.WriteString("<table class=\"attr\">")
	for _, key := range keys {
		if key != "sentid" {
			fmt.Fprintf(&buf, "<tr><td class=\"lbl\">%s</td><td>%s</td></tr>", html.EscapeString(key), html.EscapeString(fmt.Sprint(p[key])))
		}
	}
	buf.WriteString("</table>")
	return html.EscapeString(buf.String()) // dubbele escape
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

func toString(v interface{}) string {
	return fmt.Sprint(v)
}

func toInt(v interface{}) int {
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

	return nil
}

func x(err error, msg ...interface{}) bool {
	if err == nil {
		return false
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
	return true
}
