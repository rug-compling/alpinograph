package main

import (
	ag "github.com/bitnine-oss/agensgraph-golang"
	_ "github.com/lib/pq"

	"bytes"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type Node struct {
	v      ag.BasicVertex
	key    int
	id     string
	ud     *Link
	eud    []*Link
	copied string
}

type Link struct {
	from  string
	ifrom int
	rel   string
}

type ErrType struct {
	err     error
	source  string
	comment string
}

var (
	reQuote = regexp.MustCompile(`\\.`)
)

func usage() {
	fmt.Printf(`
Usage: %s [-a] corpus sentence_id...

 -a: alle zinnen

`,
		os.Args[0])
}

func cyp2ud(corpus, sentid string, comments bool) string {

	x(checkConllu(sentid))

	conllu, err := getConllu(corpus, sentid, comments)
	x(err)
	return conllu
}

func getConllu(corpus, sentid string, comments bool) (string, error) {

	var buf bytes.Buffer

	nodes := make([]*Node, 0)
	nodemap := make(map[int]*Node)
	uds := make(map[int]*Link)
	euds := make(map[int][]*Link)
	copied := make(map[int]int)

	rows, err := db.Query("match (n1)-[r:ud]->(n2:word{sentid:'" + sentid + "'}) return n1, r, n2")
	if err != nil {
		return "", wrap(err, 2)
	}

	for rows.Next() {
		var n1, r, n2 []byte
		if err := rows.Scan(&n1, &r, &n2); err != nil {
			return "", wrap(err, 1)
		}

		var v1 ag.BasicVertex
		var e ag.BasicEdge
		var v2 ag.BasicVertex

		if err := v1.Scan(n1); err != nil {
			return "", wrap(err, 1)
		}
		if err := e.Scan(r); err != nil {
			return "", wrap(err, 1)
		}
		if err := v2.Scan(n2); err != nil {
			return "", wrap(err, 1)
		}

		var ifrom, ito int
		if v1.Label == "sentence" {
			ifrom = 0
		} else {
			ifrom = toInt(v1.Properties["end"])
		}
		ito = toInt(v2.Properties["end"])
		no := Node{
			v:   v2,
			key: ito * 1000,
			id:  fmt.Sprint(ito),
		}
		nodes = append(nodes, &no)
		nodemap[no.key] = &no

		var from, to string
		if f, ok := e.Properties["from"]; ok {
			from = toString(f)
			copied[toKey(from)] = ifrom * 1000
		} else {
			from = fmt.Sprint(ifrom)
		}
		if t, ok := e.Properties["to"]; ok {
			to = toString(t)
			copied[toKey(to)] = ito * 1000
		} else {
			to = fmt.Sprint(ito)
		}
		tok := toKey(to)
		uds[tok] = &Link{
			from:  from,
			ifrom: toKey(from),
			rel:   toString(e.Properties["rel"]),
		}
	}
	if err = rows.Err(); err != nil {
		return "", wrap(err, 1)
	}

	rows, err = db.Query("match (n1)-[r:eud]->(n2:word{sentid:'" + sentid + "'}) return n1, r, n2")
	if err != nil {
		return "", wrap(err, 2)
	}

	for rows.Next() {
		var n1, r, n2 []byte
		if err := rows.Scan(&n1, &r, &n2); err != nil {
			return "", wrap(err, 1)
		}

		var v1 ag.BasicVertex
		var e ag.BasicEdge
		var v2 ag.BasicVertex

		if err := v1.Scan(n1); err != nil {
			return "", wrap(err, 1)
		}
		if err := e.Scan(r); err != nil {
			return "", wrap(err, 1)
		}
		if err := v2.Scan(n2); err != nil {
			return "", wrap(err, 1)
		}

		var ifrom, ito int
		if v1.Label == "sentence" {
			ifrom = 0
		} else {
			ifrom = toInt(v1.Properties["end"])
		}
		ito = toInt(v2.Properties["end"])

		var from, to string
		if f, ok := e.Properties["from"]; ok {
			from = toString(f)
			copied[toKey(from)] = ifrom * 1000
		} else {
			from = fmt.Sprint(ifrom)
		}
		if t, ok := e.Properties["to"]; ok {
			to = toString(t)
			copied[toKey(to)] = ito * 1000
		} else {
			to = fmt.Sprint(ito)
		}
		tok := toKey(to)
		if _, ok := euds[tok]; !ok {
			euds[tok] = make([]*Link, 0)
		}
		euds[tok] = append(euds[tok], &Link{
			from:  from,
			ifrom: toKey(from),
			rel:   toString(e.Properties["rel"])},
		)

	}
	if err := rows.Err(); err != nil {
		return "", wrap(err, 1)
	}

	for key, val := range copied {
		n1 := nodemap[val]
		nodes = append(nodes, &Node{
			v:      n1.v,
			copied: fromKey(val),
			key:    key,
			id:     fromKey(key),
		})
	}

	for i, node := range nodes {
		nodes[i].ud = uds[node.key]
		nodes[i].eud = euds[node.key]
	}

	sort.Slice(nodes, func(a, b int) bool {
		return nodes[a].key < nodes[b].key
	})

	if comments {
		fmt.Fprintf(&buf, "# corpus = %s\n# sent_id = %s\n", corpus, strings.Replace(sentid, "/", "\\", -1))

		var words bytes.Buffer
		for _, node := range nodes {
			if node.copied != "" {
				continue
			}
			words.WriteString(toString(node.v.Properties["word"]))
			if _, ok := node.v.Properties["nospaceafter"]; !ok {
				words.WriteString(" ")
			}
		}
		fmt.Fprintf(&buf, "# text = %s\n", strings.TrimSpace(words.String()))
	}

	for _, node := range nodes {
		fmt.Fprintf(&buf,
			"%s\t%s\t%s\t%s\t%s\t%s",
			node.id,
			toString(node.v.Properties["word"]),
			toString(node.v.Properties["lemma"]),
			toString(node.v.Properties["upos"]),
			attr(toString(node.v.Properties["postag"])),
			features(node.v.Properties))
		if node.ud == nil {
			fmt.Fprint(&buf, "\t_\t_")
		} else {
			fmt.Fprintf(&buf, "\t%s\t%s", node.ud.from, node.ud.rel)
		}
		sort.Slice(node.eud, func(a, b int) bool {
			return node.eud[a].ifrom < node.eud[b].ifrom
		})
		euds := make([]string, 0)
		for _, eud := range node.eud {
			euds = append(euds, eud.from+":"+eud.rel)
		}
		fmt.Fprint(&buf, "\t"+strings.Join(euds, "|"))
		if node.copied != "" {
			fmt.Fprintln(&buf, "\tCopiedFrom="+node.copied)
		} else if _, ok := node.v.Properties["nospaceafter"]; ok {
			fmt.Fprintln(&buf, "\tSpaceAfter=No")
		} else {
			fmt.Fprintln(&buf, "\t_")
		}
	}
	if comments {
		fmt.Fprintln(&buf)
	}

	return buf.String(), nil
}

func checkConllu(sentid string) error {
	rows, err := db.Query(
		"match (s:sentence{sentid: '" + sentid + "'}) return s.sentid as sid, s.tokens, s.conllu_status, s.conllu_error")
	if err != nil {
		return wrap(err, 3)
	}
	found := false
	cerr := false
	var sid, tok, status string
	var errmsg sql.NullString
	for rows.Next() {
		err := rows.Scan(&sid, &tok, &status, &errmsg)
		if err != nil {
			return wrap(err, 2)
		}
		if status == `"OK"` {
			found = true
		} else {
			cerr = true
		}
	}
	if err := rows.Err(); err != nil {
		return wrap(err, 1)
	}
	if cerr {
		return wrap(fmt.Errorf("%s: %s\n>>> %s", unescape(sid), unescape(tok), unescape(errmsg.String)), 0)
	}
	if !found {
		return wrap(fmt.Errorf("NOT FOUND: %s", sentid), 0)
	}
	return nil
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
		return -1
	case int:
		return t
	case float64:
		return int(t)
	}
	return -1
}

func toKey(s string) int {
	a := strings.Split(s, ".")
	key, _ := strconv.Atoi(a[0])
	key *= 1000

	if len(a) > 1 {
		n, err := strconv.Atoi(a[1])
		if err == nil {
			key += n
		}
	}
	return key
}

func fromKey(i int) string {
	a := i / 1000
	b := i % 1000
	if b == 0 {
		return fmt.Sprint(a)
	}
	return fmt.Sprint(a, ".", b)

}

func attr(s string) string {
	s = strings.Replace(s, "()", "", 1)
	s = strings.Replace(s, "(", "|", 1)
	s = strings.Replace(s, ")", "", 1)
	s = strings.Replace(s, ",", "|", -11)
	return s
}

func features(p map[string]interface{}) string {
	ff := make([]string, 0)
	for _, f := range []string{
		"Abbr", "Case", "Definite", "Degree", "Foreign",
		"Gender", "Number", "Person", "Poss", "PronType",
		"Reflex", "Tense", "VerbForm"} {
		if v, ok := p[f]; ok {
			ff = append(ff, f+"="+toString(v))
		}
	}
	if len(ff) == 0 {
		return "_"
	}
	return strings.Join(ff, "|")
}

func wrap(err error, offset int, msg ...interface{}) error {
	if err == nil {
		return nil
	}

	e := ErrType{err: err}

	_, filename, lineno, ok := runtime.Caller(1)
	if ok {
		e.source = fmt.Sprintf("%v:%v", filename, lineno-offset)
	}

	words := make([]string, len(msg))
	for i, m := range msg {
		words[i] = fmt.Sprint(m)
	}
	e.comment = strings.Join(words, " ")

	return &e
}

func (e *ErrType) Unwrap() error {
	return e.err
}

func (e ErrType) Error() string {
	var s, sep string

	switch e.err.(type) {
	case *ErrType:
		sep = ":\n\t"
	default:
		sep = ": "
	}

	if e.source != "" {
		s = e.source + sep + e.err.Error()
	} else {
		s = e.err.Error()
	}

	if e.comment != "" {
		return s + ", " + e.comment
	}

	return s
}
