package main

import (
	"github.com/pebbe/compactcorpus"
	"github.com/pebbe/dbxml"
	"github.com/pebbe/util"
	"github.com/rug-compling/alud/v2"

	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Alpino_ds struct {
	XMLName  xml.Name      `xml:"alpino_ds"`
	Version  string        `xml:"version,attr"`
	Metadata *MetadataType `xml:"metadata"`
	Parser   *ParserT      `xml:"parser"`
	Node0    *Node         `xml:"node"`
	Sentence *SentType     `xml:"sentence"`
}

type MetadataType struct {
	Meta []MetaT `xml:"meta"`
}

type MetaT struct {
	Type  string `xml:"type,attr"`
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type ParserT struct {
	Build string `xml:"build,attr"`
	Date  string `xml:"date,attr"`
	Cats  string `xml:"cats,attr"`
	Skips string `xml:"skips,attr"`
}

type SentType struct {
	Sent   string `xml:",chardata"`
	SentId string `xml:"sentid,attr"`
}

type Node struct {
	used         bool
	aid          string
	wordcount    int
	level        int
	parents      []*Node `xml:"node"`
	Aform        string  `xml:"aform,attr"`
	Begin        int     `xml:"begin,attr"`
	Buiging      string  `xml:"buiging,attr"`
	Case         string  `xml:"case,attr"`
	Cat          string  `xml:"cat,attr"`
	Comparative  string  `xml:"comparative,attr"`
	Conjtype     string  `xml:"conjtype,attr"`
	Def          string  `xml:"def,attr"`
	Dial         string  `xml:"dial,attr"`
	Dscmanual    string  `xml:"dscmanual,attr"`
	Dscsense     string  `xml:"dscsense,attr"`
	End          int     `xml:"end,attr"`
	Frame        string  `xml:"frame,attr"`
	Gen          string  `xml:"gen,attr"`
	Genus        string  `xml:"genus,attr"`
	Getal        string  `xml:"getal,attr"`
	GetalN       string  `xml:"getal-n,attr"` // hier een minus
	Graad        string  `xml:"graad,attr"`
	His          string  `xml:"his,attr"`
	His1         string  `xml:"his_1,attr"`
	His2         string  `xml:"his_2,attr"`
	His11        string  `xml:"his_1_1,attr"`
	His12        string  `xml:"his_1_2,attr"`
	His21        string  `xml:"his_2_1,attr"`
	His22        string  `xml:"his_2_2,attr"`
	His111       string  `xml:"his_1_1_1,attr"`
	His112       string  `xml:"his_1_1_2,attr"`
	His121       string  `xml:"his_1_2_1,attr"`
	His122       string  `xml:"his_1_2_2,attr"`
	His211       string  `xml:"his_2_1_1,attr"`
	His212       string  `xml:"his_2_1_2,attr"`
	His221       string  `xml:"his_2_2_1,attr"`
	His222       string  `xml:"his_2_2_2,attr"`
	Id           int     `xml:"id,attr"`
	Iets         string  `xml:"iets,attr"`
	Index        string  `xml:"index,attr"`
	index        int
	Infl         string  `xml:"infl,attr"`
	Lcat         string  `xml:"lcat,attr"`
	Lemma        string  `xml:"lemma,attr"`
	Lwtype       string  `xml:"lwtype,attr"`
	MwuRoot      string  `xml:"mwu_root,attr"`
	MwuSense     string  `xml:"mwu_sense,attr"`
	Naamval      string  `xml:"naamval,attr"`
	Neclass      string  `xml:"neclass,attr"`
	Npagr        string  `xml:"npagr,attr"`
	Ntype        string  `xml:"ntype,attr"`
	Num          string  `xml:"num,attr"`
	Numtype      string  `xml:"numtype,attr"`
	Pb           string  `xml:"pb,attr"`
	Pdtype       string  `xml:"pdtype,attr"`
	Per          string  `xml:"per,attr"`
	Personalized string  `xml:"personalized,attr"`
	Persoon      string  `xml:"persoon,attr"`
	Pos          string  `xml:"pos,attr"`
	Positie      string  `xml:"positie,attr"`
	Postag       string  `xml:"postag,attr"`
	Pron         string  `xml:"pron,attr"`
	Pt           string  `xml:"pt,attr"`
	Pvagr        string  `xml:"pvagr,attr"`
	Pvtijd       string  `xml:"pvtijd,attr"`
	Refl         string  `xml:"refl,attr"`
	Rel          string  `xml:"rel,attr"`
	Rnum         string  `xml:"rnum,attr"`
	Root         string  `xml:"root,attr"`
	Sc           string  `xml:"sc,attr"`
	Sense        string  `xml:"sense,attr"`
	Special      string  `xml:"special,attr"`
	Spectype     string  `xml:"spectype,attr"`
	Status       string  `xml:"status,attr"`
	Stype        string  `xml:"stype,attr"`
	Tense        string  `xml:"tense,attr"`
	Vform        string  `xml:"vform,attr"`
	Vwtype       string  `xml:"vwtype,attr"`
	Vztype       string  `xml:"vztype,attr"`
	Wh           string  `xml:"wh,attr"`
	Wk           string  `xml:"wk,attr"`
	Word         string  `xml:"word,attr"`
	Wvorm        string  `xml:"wvorm,attr"`
	NodeList     []*Node `xml:"node"`
}

// Een dependency relation
type Deprel struct {
	word, lemma, rel         string
	hword, hlemma, hrel      string
	begin, hbegin, end, hend int
	id, hid                  int
}

// Een node met het pad naar de node
type NodePath struct {
	node *Node
	path []string
}

type Doc struct {
	name string
	data []byte
}

const (
	idSentence = 3
	// Nw is nummer 4, wordt niet gebruikt
	idNode = 5
	idWord = 6
	idMeta = 7
	// Dep is nummer 8, wordt niet gebruikt
	idUd   = 9
	idEud  = 10
	idRel  = 11
	idNext = 12
	idPair = 13
)

var (
	x         = util.CheckErr
	reCopied  = regexp.MustCompile(`CopiedFrom=([0-9]+)`)
	reSpecial = regexp.MustCompile(`["\n\\]`)

	refnodes []*Node   // reset per zin
	deprels  []*Deprel // reset per zin

	targets = []string{"hd", "cmp", "crd", "dlink", "rhd", "whd"}

	opt_t = flag.String("t", "", "title")

	tmp     string
	current string

	lblSentence string

	nSentence int
	nNode     int
	nWord     int
	nMeta     int
	nUd       int
	nEud      int
	nRel      int
	nNext     int
	nPair     int

	fpSentence *os.File
	fpNode     *os.File
	fpWord     *os.File
	fpMeta     *os.File
	fpUd       *os.File
	fpEud      *os.File
	fpRel      *os.File
	fpNext     *os.File
	fpPair     *os.File

	chDoc  = make(chan Doc)
	chArch = make(chan string)

	featureMap = map[string]map[string]int{
		"meta": make(map[string]int),
		"node": make(map[string]int),
		"word": make(map[string]int),
	}
)

func usage() {
	fmt.Fprintf(os.Stderr, `
Usage: %s [-t title] file.(dact|index|data.dz) [file...]

Option -t is required if there is more than one input file.

`, os.Args[0])
}

func main() {

	flag.Usage = usage
	flag.Parse()

	if flag.NArg() == 0 || (flag.NArg() > 1 && *opt_t == "") {
		usage()
		return
	}

	corpus := *opt_t
	if corpus == "" {
		corpus = basename(flag.Arg(0))
	}

	tmp = "tmp." + corpus + "."

	var err error
	fpSentence, err = os.Create(tmp + "sentence")
	x(err)
	fpNode, err = os.Create(tmp + "node")
	x(err)
	fpWord, err = os.Create(tmp + "word")
	x(err)
	fpMeta, err = os.Create(tmp + "meta")
	x(err)
	fpUd, err = os.Create(tmp + "ud")
	x(err)
	fpEud, err = os.Create(tmp + "eud")
	x(err)
	fpRel, err = os.Create(tmp + "rel")
	x(err)
	fpNext, err = os.Create(tmp + "next")
	x(err)
	fpPair, err = os.Create(tmp + "pair")
	x(err)

	fmt.Fprintf(fpSentence, "COPY %s.sentence (id, properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpNode, "COPY %s.node (id, properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpWord, "COPY %s.word (id, properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpMeta, "COPY %s.meta (id, properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpUd, "COPY %s.ud (id, start, \"end\", properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpEud, "COPY %s.eud (id, start, \"end\", properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpRel, "COPY %s.rel (id, start, \"end\", properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpNext, "COPY %s.next (id, start, \"end\", properties) FROM stdin;\n", corpus)
	fmt.Fprintf(fpPair, "COPY %s.pair (id, start, \"end\", properties) FROM stdin;\n", corpus)

	// prefase

	fmt.Printf(`drop graph if exists %s cascade;
create graph %s;
set graph_path='%s';
create vlabel sentence;
create vlabel nw;
create vlabel node inherits (nw);
create vlabel word inherits (nw);
create vlabel meta;
create elabel dep;
create elabel ud inherits (dep);
create elabel eud inherits (dep);
create elabel rel;
create elabel next;
create elabel pair;
`, corpus, corpus, corpus)

	seen := make(map[string]bool)
	hasMeta := false

	go getFiles()

	count := 0
	idx := 0
LOOP:
	for {
		var doc Doc
		var ok bool
		select {
		case doc, ok = <-chDoc:
			if !ok {
				break LOOP
			}
		case current = <-chArch:
			idx++
			count = 0
			continue LOOP
		}
		count++

		fmt.Fprintf(os.Stderr, "  %d:%d %s                \r", idx, count, doc.name)

		var alpino Alpino_ds
		x(xml.Unmarshal(doc.data, &alpino))

		/*
		   Zoek alle referenties. Dit zijn nodes met een attribuut "index".
		   Sla deze op in een tabel: index -> *Node
		   Dit moet VOOR mwu() omdat anders somige indexnodes niet meer beschikbaar zijn
		*/
		refnodes = make([]*Node, len(strings.Fields(alpino.Sentence.Sent)))
		prepare(alpino.Node0, 0)
		addIndexParents(alpino.Node0)
		wordcount(alpino.Node0)

		var f func(*Node)
		f = func(node *Node) {
			if node.Cat != "" {
				nNode++
				node.aid = fmt.Sprintf("%d.%d", idNode, nNode)
				if node.NodeList != nil {
					for _, n := range node.NodeList {
						f(n)
					}
				}
			} else if node.Word != "" {
				nWord++
				node.aid = fmt.Sprintf("%d.%d", idWord, nWord)
			}
		}
		f(alpino.Node0)

		sentid := alpino.Sentence.SentId
		if sentid == "" {
			sentid = strings.Replace(doc.name, ".xml", "", 1)
		}

		if seen[sentid] {
			x(fmt.Errorf("Duplicate sentid %q in %s", sentid, current))
		}
		seen[sentid] = true

		var buf bytes.Buffer
		if alpino.Parser != nil {
			if ct := alpino.Parser.Cats; ct != "" {
				fmt.Fprintf(&buf, `, "cats": %s`, ct)
			}
			if sk := alpino.Parser.Skips; sk != "" {
				fmt.Fprintf(&buf, `, "skips": %s`, sk)
			}
			if bl := alpino.Parser.Build; bl != "" {
				fmt.Fprintf(&buf, `, "build": %q`, bl)
			}
			if dt := alpino.Parser.Date; dt != "" {
				fmt.Fprintf(&buf, `, "date": %q`, dt)
			}
		}

		s, err := alud.Ud(doc.data, doc.name, "", 0)
		conlluErr := ""
		conlluLines := make([][]string, 0)
		text := ""
		if err != nil {
			conlluErr = err.Error()
		} else {
			for _, line := range strings.Split(s, "\n") {
				if strings.HasPrefix(line, "# text = ") {
					text = strings.TrimSpace(line[8:])
				} else if a := strings.Split(line, "\t"); len(a) == 10 {
					conlluLines = append(conlluLines, a)
				}
			}
		}

		// conllu
		feats := make([]string, alpino.Node0.End)
		if len(conlluLines) > 0 {
			for _, aa := range conlluLines {
				if !strings.Contains(aa[0], ".") {
					idx, _ := strconv.Atoi(aa[0])
					var buf2 bytes.Buffer
					fmt.Fprintf(&buf2, `, "upos": %s`, q(aa[3]))
					if aa[5] != "_" {
						for _, item := range strings.Split(aa[5], "|") {
							ab := strings.SplitN(item, "=", 2)
							fmt.Fprintf(&buf2, `, "%s": %s`, ab[0], q(ab[1]))
						}
					}
					if strings.Contains(aa[9], "SpaceAfter=No") {
						buf2.WriteString(`, "nospaceafter": true`)
					}
					feats[idx-1] = buf2.String()
				}
			}
		}

		if conlluErr == "" {
			fmt.Fprintf(&buf, `, "text": %s, "conllu_status": "OK"`, q(text))
		} else {
			fmt.Fprintf(&buf, `, "conllu_status": "error", "conllu_error": %s`, q(conlluErr))
		}

		nSentence++
		lblSentence = fmt.Sprintf("%d.%d", idSentence, nSentence)
		fmt.Fprintf(fpSentence, "%s\t{\"sentid\": %s, \"tokens\": %s, \"len\": %d%s}\n",
			lblSentence,
			q(sentid), q(alpino.Sentence.Sent), alpino.Node0.End, buf.String())

		nRel++
		fmt.Fprintf(fpRel, "%d.%d\t%s\t%s\t{\"rel\": %s}\n",
			idRel, nRel,
			lblSentence,
			alpino.Node0.aid,
			q(alpino.Node0.Rel))

		doNode1(sentid, alpino.Node0, alpino.Node0.End, feats)

		if alpino.Metadata != nil && alpino.Metadata.Meta != nil && len(alpino.Metadata.Meta) > 0 {
			hasMeta = true
			for _, m := range alpino.Metadata.Meta {
				nMeta++
				fmt.Fprintf(fpMeta, "%d.%d\t{\"sentid\": %s, \"name\": %s, \"type\": %s, \"value\": %s}\n",
					idMeta, nMeta,
					q(sentid),
					q(m.Name),
					q(m.Type),
					qt(m.Value, m.Type))
				featureMap["meta"][m.Name] = featureMap["meta"][m.Name] + 1
			}
		}

		doNode2(alpino.Node0)

		indexen := make(map[string]string)
		words := make([]string, alpino.Node0.End)
		f = func(node *Node) {
			if node.Word != "" {
				words[node.Begin] = node.aid
			}
			if node.Index != "" && (node.Word != "" || node.Cat != "") {
				indexen[node.Index] = node.aid
			}
			if node.NodeList != nil {
				for _, n := range node.NodeList {
					f(n)
				}
			}
		}
		f(alpino.Node0)
		if len(indexen) > 0 {
			f = func(node *Node) {
				if node.NodeList != nil {
					for _, n := range node.NodeList {
						if n.Index != "" && n.Word == "" && n.Cat == "" {
							nRel++
							fmt.Fprintf(fpRel, "%d.%d\t%s\t%s\t{\"rel\": %s, \"id\": %d}\n",
								idRel, nRel,
								node.aid,
								indexen[n.Index],
								q(n.Rel),
								n.Id)
						}
						f(n)
					}
				}
			}
			f(alpino.Node0)
		}

		for i := 0; i < alpino.Node0.End-1; i++ {
			nNext++
			fmt.Fprintf(fpNext, "%d.%d\t%s\t%s\t{}\n",
				idNext, nNext,
				words[i], words[i+1])
		}

		// conllu
		if len(conlluLines) > 0 {
			extra := make(map[string]string)
			for _, aa := range conlluLines {
				if strings.Contains(aa[0], ".") {
					m := reCopied.FindStringSubmatch(aa[9])
					extra[aa[0]] = m[1]
				}
			}
			for _, aa := range conlluLines {
				if !strings.Contains(aa[0], ".") {
					idx0, _ := strconv.Atoi(aa[0])
					idx6, _ := strconv.Atoi(aa[6])
					ma := ""
					if rels := strings.SplitN(aa[7], ":", 2); len(rels) == 2 {
						ma = fmt.Sprintf("\"main\": %s, \"aux\": %s", q(rels[0]), q(rels[1]))
					} else {
						ma = fmt.Sprintf("\"main\": %s", q(rels[0]))
					}

					// basic
					if aa[6] == "0" {
						nUd++
						fmt.Fprintf(fpUd, "%d.%d\t%s\t%s\t{\"rel\": %s, %s}\n",
							idUd, nUd,
							lblSentence,
							words[idx0-1],
							q(aa[7]), ma)
					} else {
						nUd++
						fmt.Fprintf(fpUd, "%d.%d\t%s\t%s\t{\"rel\": %s, %s}\n",
							idUd, nUd,
							words[idx6-1],
							words[idx0-1],
							q(aa[7]), ma)
					}
				}
				// enhanced
				dst := aa[0]
				to := ""
				if strings.Contains(dst, ".") {
					to = fmt.Sprintf(", \"to\": %s", q(dst))
					dst = extra[dst]
				}
				dsti, _ := strconv.Atoi(dst)
				for _, enh := range strings.Split(aa[8], "|") {
					abc := strings.SplitN(enh, ":", 3)
					src := abc[0]
					from := ""
					if strings.Contains(src, ".") {
						from = fmt.Sprintf(", \"from\": %s", q(src))
						src = extra[src]
					}
					srci, _ := strconv.Atoi(src)
					aux := ""
					if len(abc) == 3 {
						aux = fmt.Sprintf(", \"aux\": %s", q(abc[2]))
					}
					if src == "0" {
						nEud++
						fmt.Fprintf(fpEud, "%d.%d\t%s\t%s\t{\"rel\": %s, \"main\": %s%s%s%s}\n",
							idEud, nEud,
							lblSentence,
							words[dsti-1],
							q(strings.Join(abc[1:], ":")), q(abc[1]), aux, to, from)
					} else {
						nEud++
						fmt.Fprintf(fpEud, "%d.%d\t%s\t%s\t{\"rel\": %s, \"main\": %s%s%s%s}\n",
							idEud, nEud,
							words[srci-1],
							words[dsti-1],
							q(strings.Join(abc[1:], ":")), q(abc[1]), aux, to, from)
					}
				}
			}
		}

		doParen(&alpino)
	}

	// eindfase

	fpSentence.Close()
	fpNode.Close()
	fpWord.Close()
	fpMeta.Close()
	fpUd.Close()
	fpEud.Close()
	fpRel.Close()
	fpNext.Close()
	fpPair.Close()

	for _, f := range []string{"sentence", "node", "word", "meta", "ud", "eud", "rel", "next", "pair"} {
		if hasMeta || f != "meta" {
			fp, err := os.Open(tmp + f)
			x(err)
			_, err = io.Copy(os.Stdout, fp)
			x(err)
			fp.Close()
			fmt.Print("\\.\n\n")
		}
		os.Remove(tmp + f)
	}

	fmt.Printf("select pg_catalog.setval('%s.sentence_id_seq', %d, true);\n", corpus, nSentence)
	fmt.Printf("select pg_catalog.setval('%s.node_id_seq', %d, true);\n", corpus, nNode)
	fmt.Printf("select pg_catalog.setval('%s.word_id_seq', %d, true);\n", corpus, nWord)
	if hasMeta {
		fmt.Printf("select pg_catalog.setval('%s.meta_id_seq', %d, true);\n", corpus, nMeta)
	}
	fmt.Printf("select pg_catalog.setval('%s.ud_id_seq', %d, true);\n", corpus, nUd)
	fmt.Printf("select pg_catalog.setval('%s.eud_id_seq', %d, true);\n", corpus, nEud)
	fmt.Printf("select pg_catalog.setval('%s.rel_id_seq', %d, true);\n", corpus, nRel)
	fmt.Printf("select pg_catalog.setval('%s.next_id_seq', %d, true);\n", corpus, nNext)
	fmt.Printf("select pg_catalog.setval('%s.pair_id_seq', %d, true);\n", corpus, nPair)

	for nw, features := range featureMap {
		for key, value := range features {
			fmt.Printf("CREATE (:feature{v: '%s', name: '%s', count: %d});\n", nw, sq(key), value)
		}
	}

	fmt.Print(`create property index on sentence("sentid");
create property index on sentence("cats");
create property index on sentence("skips");
create property index on word("cat");
create property index on word("graad");
create property index on word("lemma");
create property index on word("pt");
create property index on word("root");
create property index on word("sentid");
create property index on word("upos");
create property index on word("word");
create property index on word("neclass");
create property index on word("his");
create property index on word("_clause");
create property index on word("_clause_lvl");
create property index on word("_deste");
create property index on word("_np");
create property index on word("_vorfeld");
create property index on node("cat");
create property index on node("graad");
create property index on node("lemma");
create property index on node("pt");
create property index on node("root");
create property index on node("sentid");
create property index on node("upos");
create property index on node("word");
create property index on node("neclass");
create property index on node("his");
create property index on node("_clause");
create property index on node("_clause_lvl");
create property index on node("_deste");
create property index on node("_np");
create property index on node("_vorfeld");
create property index on rel("id");
create property index on rel("rel");
create property index on pair("rel");
create property index on ud("rel");
create property index on ud("main");
create property index on eud("rel");
create property index on eud("main");
`)
	if hasMeta {
		fmt.Print(`
create property index on meta("sentid");
create property index on meta("name");
create property index on meta("value");
`)
	}

	fmt.Printf(`create (:doc{alud_version: '%s'});
GRANT USAGE ON SCHEMA %s TO guest;
GRANT SELECT ON ALL TABLES IN SCHEMA %s TO guest;
ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT SELECT ON TABLES TO guest;
`, alud.VersionID(), corpus, corpus, corpus)

}

func doNode1(sentid string, node *Node, last int, feats []string) {
	if node.Cat != "" {
		var mwu string
		if validMwu(node) {
			lemma := make([]string, 0)
			word := make([]string, 0)
			p := 0
			for i, n := range node.NodeList {
				if i > 0 {
					if n.Begin != p {
						lemma = append(lemma, "[...]")
						word = append(word, "[...]")
					}
				}
				p = n.End

				// TODO
				if n.Word == "" && len(n.NodeList) == 0 && n.Index != "" {
					n = refnodes[n.index]
				}
				lemma = append(lemma, n.Lemma)
				word = append(word, n.Word)
			}
			mwu = fmt.Sprintf(", \"lemma\": %s, \"pt\": \"mwu\", \"word\": %s",
				q(strings.Join(lemma, " ")),
				q(strings.Join(word, " ")))
		}
		var buf bytes.Buffer
		for _, name := range NodeTags {
			if val := getAttr(name, node); val != "" {
				fmt.Fprintf(&buf, `, "%s": %s`, name, q(val))
			}
		}
		lvl := ""
		if node.level > 0 {
			lvl = fmt.Sprintf(`, "_clause": true, "_clause_lvl": %d`, node.level)
		}
		np := ""
		if isNP(node) {
			np = `, "_np": true`
		}
		vorfeld := ""
		if isVorfeld(node) {
			vorfeld = `, "_vorfeld": true`
		}
		deste := ""
		if isDeste(node) {
			deste = `, "_deste": true`
		}
		jsn := fmt.Sprintf("{\"sentid\": %s, \"id\": %d, \"begin\": %d, \"end\": %d, \"_n_words\": %d%s%s%s%s%s%s}",
			q(sentid),
			node.Id,
			node.Begin,
			node.End,
			node.wordcount,
			buf.String(),
			mwu,
			lvl,
			np,
			vorfeld,
			deste)
		fmt.Fprintf(fpNode, "%s\t%s\n", node.aid, jsn)
		featureCount("node", jsn)
		if node.NodeList != nil {
			for _, n := range node.NodeList {
				doNode1(sentid, n, last, feats)
			}
		}
		return
	}
	if node.Word != "" {
		var buf bytes.Buffer
		for _, name := range NodeTags {
			if val := getAttr(name, node); val != "" {
				fmt.Fprintf(&buf, `, "%s": %s`, name, q(val))
			}
		}
		if node.End == last {
			fmt.Fprint(&buf, ", \"last\": true")
		}
		np := ""
		if isNP(node) {
			np = `, "_np": true`
		}
		vorfeld := ""
		if isVorfeld(node) {
			vorfeld = `, "_vorfeld": true`
		}
		jsn := fmt.Sprintf("{\"sentid\": %s, \"id\": %d, \"begin\": %d, \"end\": %d, \"_n_words\": %d%s%s%s%s}",
			q(sentid),
			node.Id,
			node.Begin,
			node.End,
			node.wordcount,
			buf.String(),
			feats[node.Begin],
			np,
			vorfeld)
		fmt.Fprintf(fpWord, "%s\t%s\n", node.aid, jsn)
		featureCount("word", jsn)
	}
}

func doNode2(node *Node) {
	for _, n := range node.NodeList {
		if n.Cat != "" {
			nRel++
			fmt.Fprintf(fpRel, "%d.%d\t%s\t%s\t{\"rel\": %s}\n",
				idRel, nRel,
				node.aid,
				n.aid,
				q(n.Rel))
			doNode2(n)
			continue
		}
		if n.Word != "" {
			nRel++
			fmt.Fprintf(fpRel, "%d.%d\t%s\t%s\t{\"rel\": %s}\n",
				idRel, nRel,
				node.aid,
				n.aid,
				q(n.Rel))
		}
	}
}

var NodeTags = []string{
	"aform",
	// "begin", // al gedaan
	"buiging",
	"case",
	"cat",
	"comparative",
	"conjtype",
	"def",
	"dial",
	"dscmanual",
	"dscsense",
	// "end", // al gedaan
	"frame",
	"gen",
	"genus",
	"getal",
	"getal_n", // hier een underscore
	"graad",
	"his",
	"his_1",
	"his_2",
	"his_1_1",
	"his_1_2",
	"his_2_1",
	"his_2_2",
	"his_1_1_1",
	"his_1_1_2",
	"his_1_2_1",
	"his_1_2_2",
	"his_2_1_1",
	"his_2_1_2",
	"his_2_2_1",
	"his_2_2_2",
	// "id", // al gedaan
	"iets",
	// "index", // al gedaan
	"infl",
	"lcat",
	"lemma",
	"lwtype",
	"mwu_root",
	"mwu_sense",
	"naamval",
	"neclass",
	"npagr",
	"ntype",
	"num",
	"numtype",
	"other_id",
	"pb",
	"pdtype",
	"per",
	"personalized",
	"persoon",
	"pos",
	"positie",
	"postag",
	"pron",
	"pt",
	"pvagr",
	"pvtijd",
	"refl",
	// "rel", // al gedaan
	"rnum",
	"root",
	"sc",
	"sense",
	"special",
	"spectype",
	"status",
	"stype",
	"tense",
	"vform",
	"vwtype",
	"vztype",
	"wh",
	"wk",
	"word",
	"wvorm",
}

func getAttr(attr string, n *Node) string {
	switch attr {
	case "aform":
		return n.Aform
	case "buiging":
		return n.Buiging
	case "case":
		return n.Case
	case "cat":
		return n.Cat
	case "comparative":
		return n.Comparative
	case "conjtype":
		return n.Conjtype
	case "def":
		return n.Def
	case "dial":
		return n.Dial
	case "dscmanual":
		return n.Dscmanual
	case "dscsense":
		return n.Dscsense
	case "frame":
		return n.Frame
	case "gen":
		return n.Gen
	case "genus":
		return n.Genus
	case "getal":
		return n.Getal
	case "getal_n": // hier een underscore
		return n.GetalN
	case "graad":
		return n.Graad
	case "his":
		return n.His
	case "his_1":
		return n.His1
	case "his_2":
		return n.His2
	case "his_1_1":
		return n.His11
	case "his_1_2":
		return n.His12
	case "his_2_1":
		return n.His21
	case "his_2_2":
		return n.His22
	case "his_1_1_1":
		return n.His111
	case "his_1_1_2":
		return n.His112
	case "his_1_2_1":
		return n.His121
	case "his_1_2_2":
		return n.His122
	case "his_2_1_1":
		return n.His211
	case "his_2_1_2":
		return n.His212
	case "his_2_2_1":
		return n.His221
	case "his_2_2_2":
		return n.His222
	case "iets":
		return n.Iets
	case "index":
		return n.Index
	case "infl":
		return n.Infl
	case "lcat":
		return n.Lcat
	case "lemma":
		return n.Lemma
	case "lwtype":
		return n.Lwtype
	case "mwu_root":
		return n.MwuRoot
	case "mwu_sense":
		return n.MwuSense
	case "naamval":
		return n.Naamval
	case "neclass":
		return n.Neclass
	case "npagr":
		return n.Npagr
	case "ntype":
		return n.Ntype
	case "num":
		return n.Num
	case "numtype":
		return n.Numtype
	case "pb":
		return n.Pb
	case "pdtype":
		return n.Pdtype
	case "per":
		return n.Per
	case "personalized":
		return n.Personalized
	case "persoon":
		return n.Persoon
	case "pos":
		return n.Pos
	case "positie":
		return n.Positie
	case "postag":
		return n.Postag
	case "pron":
		return n.Pron
	case "pt":
		return n.Pt
	case "pvagr":
		return n.Pvagr
	case "pvtijd":
		return n.Pvtijd
	case "refl":
		return n.Refl
	case "rel":
		return n.Rel
	case "rnum":
		return n.Rnum
	case "root":
		return n.Root
	case "sc":
		return n.Sc
	case "sense":
		return n.Sense
	case "special":
		return n.Special
	case "spectype":
		return n.Spectype
	case "status":
		return n.Status
	case "stype":
		return n.Stype
	case "tense":
		return n.Tense
	case "vform":
		return n.Vform
	case "vwtype":
		return n.Vwtype
	case "vztype":
		return n.Vztype
	case "wh":
		return n.Wh
	case "wk":
		return n.Wk
	case "word":
		return n.Word
	case "wvorm":
		return n.Wvorm
	}
	return ""
}

func doParen(alpino *Alpino_ds) {

	// multi-word units "ineenvouwen"
	mwu(alpino.Node0)

	/*
	   Zoek alle dependency relations, en sla die op in de tabel 'deprels'
	*/
	deprels = make([]*Deprel, 0, len(strings.Fields(alpino.Sentence.Sent)))
	traverse(alpino.Node0)

	/*
	   Sla alle resterende woorden, waarvoor geen dependency relation is gevonden op in de tabel 'deprels'.
	*/
	traverse2(alpino.Node0)

	seen := make(map[string]bool)
	for _, deprel := range deprels {
		var rel string
		if deprel.hrel == "" {
			rel = deprel.rel + "/-"
		} else if deprel.hrel == "hd" {
			rel = deprel.rel
		} else {
			rel = deprel.rel + "/" + deprel.hrel
		}
		if deprel.hid < 0 {
			aid := getAid(alpino.Node0, deprel.id)
			if aid == "" {
				util.WarnErr(fmt.Errorf("Missing aid for %s: %d", alpino.Sentence.SentId, deprel.id))
			} else {
				key := fmt.Sprintf("%s\t%s\t{\"rel\": %s}", lblSentence, aid, q(rel))
				if !seen[key] {
					seen[key] = true
					nPair++
					fmt.Fprintf(fpPair, "%d.%d\t%s\n",
						idPair, nPair, key)
				}
			}
		} else {
			id1 := getAid(alpino.Node0, deprel.hid)
			id2 := getAid(alpino.Node0, deprel.id)
			if id1 == "" {
				util.WarnErr(fmt.Errorf("Missing aid for %s: %d", alpino.Sentence.SentId, deprel.hid))
			}
			if id2 == "" {
				util.WarnErr(fmt.Errorf("Missing aid for %s: %d", alpino.Sentence.SentId, deprel.id))
			}
			if id1 != "" && id2 != "" {
				if id1 != id2 {
					key := fmt.Sprintf("%s\t%s\t{\"rel\": %s}", id1, id2, q(rel))
					if !seen[key] {
						seen[key] = true
						nPair++
						fmt.Fprintf(fpPair, "%d.%d\t%s\n", idPair, nPair, key)
					}
				}
			}
		}
	}
}

// Multi-word units "ineenvouwen".
func mwu(node *Node) {
	if validMwu(node) {
		node.Postag = node.Cat
		node.Cat = ""
		words := make([]string, 0, node.End-node.Begin)
		lemmas := make([]string, 0, node.End-node.Begin)
		roots := make([]string, 0, node.End-node.Begin)
		for _, n := range node.NodeList {
			words = append(words, n.Word)
			lemmas = append(lemmas, n.Lemma)
			roots = append(roots, n.Root)
		}
		node.Word = strings.Join(words, " ")
		node.Lemma = strings.Join(lemmas, " ")
		node.Root = strings.Join(roots, " ")
		node.NodeList = node.NodeList[0:0]
	}
	for _, n := range node.NodeList {
		mwu(n)
	}
}

// Zoek alle referenties. Dit zijn nodes met een attribuut "index".
// Sla deze op in een tabel 'refnames': index -> *Node
func prepare(node *Node, level int) {
	if node.Cat == "smain" || node.Cat == "sv1" || node.Cat == "ssub" {
		level++
		node.level = level
	}
	if node.parents == nil {
		node.parents = make([]*Node, 0)
	}
	if node.Index != "" {
		node.index, _ = strconv.Atoi(node.Index)
		if len(node.Word) != 0 || len(node.NodeList) != 0 {
			for len(refnodes) <= node.index {
				refnodes = append(refnodes, nil)
			}
			refnodes[node.index] = node
		}
	}
	for _, n := range node.NodeList {
		if n.parents == nil {
			n.parents = make([]*Node, 0)
		}
		n.parents = append(n.parents, node)
		prepare(n, level)
	}
}

func addIndexParents(node *Node) {
	for _, n := range node.NodeList {
		if n.index > 0 {
			if nn := refnodes[n.index]; nn != n {
				nn.parents = append(nn.parents, node)
			}
		}
		addIndexParents(n)
	}
}

func wordcount(node *Node) map[int]bool {
	m := make(map[int]bool)
	if node.Word == "" {
		if node.NodeList != nil {
			for _, n := range node.NodeList {
				if n.index > 0 {
					n = refnodes[n.index]
				}
				for key := range wordcount(n) {
					m[key] = true
				}
			}
		}
		node.wordcount = len(m)
	} else {
		node.wordcount = 1
		m[node.Id] = true
	}
	return m
}

// Zoek alle dependency relations, en sla die op in de tabel 'deprels'
func traverse(node *Node) {
	if len(node.NodeList) == 0 {
		return
	}

	// Zoek hoofd-dochter. Dit is de eerste van 'targets': hd cmp crd dlink rhd whd
	idx := -1
TARGET:
	for _, target := range targets {
		for i, n := range node.NodeList {
			if n.Rel == target {
				idx = i
				break TARGET
			}
		}
	}
	if idx >= 0 {
		heads := find_head(node.NodeList[idx])
		for i, n := range node.NodeList {
			if i == idx {
				continue
			}
			for _, np2 := range find_head(n) {
				n2 := np2.node
				for _, headpath := range heads {
					head := headpath.node
					lassy_deprel(n2.Word, n2.Lemma, n.Rel, // n.Rel, dus niet n2.Rel !
						head.Word, head.Lemma, node.NodeList[idx].Rel,
						n2.Begin, n2.End, head.Begin, head.End, n2.Id, head.Id)
					n2.used = true
				}
			}
		}
	}

	// Zoek su-dochter met obj1-dochter of obj2-dochter
	idx = -1
	for i, n := range node.NodeList {
		if n.Rel == "su" {
			idx = i
			break
		}
	}
	if idx >= 0 {
		subjs := find_head(node.NodeList[idx])
		for _, obj := range node.NodeList {
			if obj.Rel != "obj1" && obj.Rel != "obj2" {
				continue
			}
			for _, op := range find_head(obj) {
				o := op.node
				for _, sup := range subjs {
					su := sup.node
					lassy_deprel(o.Word, o.Lemma, obj.Rel,
						su.Word, su.Lemma, "su",
						o.Begin, o.End, su.Begin, su.End, o.Id, su.Id)
					o.used = true
					lassy_deprel(su.Word, su.Lemma, "su",
						o.Word, o.Lemma, obj.Rel,
						su.Begin, su.End, o.Begin, o.End, su.Id, o.Id)
					su.used = true
				}
			}
		}
	}

	// cat conj: alles kan head zijn
	if node.Cat == "conj" {
		heads := make([][]*NodePath, len(node.NodeList))
		for i, n1 := range node.NodeList {
			heads[i] = find_head(n1)
		}
		for i := 1; i < len(heads); i++ {
			for j := 0; j < i; j++ {
				for _, np1 := range heads[i] {
					n1 := np1.node
					for _, np2 := range heads[j] {
						n2 := np2.node
						lassy_deprel(n1.Word, n1.Lemma, node.NodeList[i].Rel,
							n2.Word, n2.Lemma, node.NodeList[j].Rel,
							n1.Begin, n1.End, n2.Begin, n2.End, n1.Id, n2.Id)
						n1.used = true
						lassy_deprel(n2.Word, n2.Lemma, node.NodeList[j].Rel,
							n1.Word, n1.Lemma, node.NodeList[i].Rel,
							n2.Begin, n2.End, n1.Begin, n1.End, n2.Id, n1.Id)
						n2.used = true
					}
				}
			}
		}
	}

	for _, n := range node.NodeList {
		traverse(n)
	}

}

// Sla alle resterende woorden, waarvoor geen dependency relation is gevonden op in de tabel 'deprels'.
func traverse2(node *Node) {
	// negeer woorden met relatie == "--" en pt == "let"
	if node.Word != "" && !(node.Rel == "--" && node.Postag == "let") && !node.used {
		lassy_deprel(node.Word, node.Lemma, node.Rel,
			"", "", "", node.Begin, node.End, 0, 0, node.Id, -1)
	}
	for _, n := range node.NodeList {
		traverse2(n)
	}
}

// Geef een lijst van alle dochters van node die als head kunnen optreden.
// Bij elke dochter, geef ook het pad dat naar die node leidde.
func find_head(node *Node) []*NodePath {
	path := []string{fmt.Sprint(node.Id)}

	/*
		Als we bij een index zijn, spring naar de node met de definitie voor deze index.
		(Dat kan de node zelf zijn.)
		De node waarnaar gesprongen wordt wordt niet opgenomen in het pad. Dat is iets
		wat in het programma lassytree opgelost moet worden. Wel opnemen in het pad zorgt
		voor problemen die in het programma lassytree veel moeilijker zijn op te lossen.
	*/
	if node.index > 0 {
		n := refnodes[node.index]
		if n == nil {
			x(fmt.Errorf("Missing refnode for index=%s, id=%d in %s", node.Index, node.Id, current))
		}
		node = n
	}

	/*
		Als het woord niet leeg is, dan hebben we een terminal bereikt.
	*/
	if node.Word != "" {
		// negeer woorden met relatie == "--" en pt == "let"
		if node.Rel == "--" && node.Postag == "let" {
			return []*NodePath{}
		}
		return []*NodePath{&NodePath{node: node, path: path}}
	}

	/*
		Als de node categorie "conj" heeft, dan kan elke dochter een head zijn.
		Geef een lijst van de heads van alle dochters.
	*/
	if node.Cat == "conj" {
		nodes := make([]*NodePath, 0, len(node.NodeList))
		for _, n := range node.NodeList {
			for _, n2 := range find_head(n) {
				p2 := make([]string, len(n2.path))
				copy(p2, n2.path)
				for _, p := range path {
					p2 = append(p2, p)
				}
				nodes = append(nodes, &NodePath{node: n2.node, path: p2})
			}
		}
		return nodes
	}

	/*
		Zoek hoofd-dochter. Dit is de eerste van 'targets': hd cmp crd dlink rhd whd
	*/
	for _, target := range targets {
		for _, n := range node.NodeList {
			if n.Rel == target {
				nodes := make([]*NodePath, 0)
				for _, n2 := range find_head(n) {
					p2 := make([]string, len(n2.path))
					copy(p2, n2.path)
					for _, p := range path {
						p2 = append(p2, p)
					}
					nodes = append(nodes, &NodePath{node: n2.node, path: p2})
				}
				return nodes
			}
		}
	}

	// Geen hoofd gevonden: retourneer lege lijst
	return []*NodePath{}
}

// Voeg een dependency relation toe aan de lijst 'deprels'
func lassy_deprel(word, lemma, rel, hword, hlemma, hrel string, begin, end, hbegin, hend, id, hid int) {
	deprels = append(deprels, &Deprel{
		word:   word,
		lemma:  lemma,
		rel:    rel,
		hword:  hword,
		hlemma: hlemma,
		hrel:   hrel,
		begin:  begin,
		end:    end,
		hbegin: hbegin,
		hend:   hend,
		id:     id,
		hid:    hid,
	})
}

func getAid(node *Node, id int) string {
	if node.Id == id {
		if node.Index != "" {
			node = refnodes[node.index]
		}
		return node.aid
	}
	if node.Index != "" && refnodes[node.index] != node {
		return getAid(refnodes[node.index], id)
	}
	if node.NodeList != nil {
		for _, n := range node.NodeList {
			if s := getAid(n, id); s != "" {
				return s
			}
		}
	}
	return ""
}

func q(s string) string {
	return `"` + reSpecial.ReplaceAllStringFunc(s, func(s1 string) string {
		if s1 == "\n" {
			return `\\n`
		}
		if s1 == `"` {
			return `\\"`
		}
		if s1 == `\` {
			return `\\\\`
		}
		x(fmt.Errorf("shouldn't happen"))
		return s1
	}) + `"`
}

func qt(s string, stype string) string {
	if stype == "int" || stype == "float" {
		return s
	}
	return q(s)
}

func nodeExtra(node *Node) string {
	var buf bytes.Buffer

	if node.MwuRoot != "" {
		fmt.Fprintf(&buf, ", \"mwu_root\": %s", q(node.MwuRoot))
	}
	if node.MwuSense != "" {
		fmt.Fprintf(&buf, ", \"mwu_sense\": %s", q(node.MwuSense))
	}
	for _, tag := range NodeTags {
		if strings.HasPrefix(tag, "his") {
			if value := getAttr(tag, node); value != "" {
				fmt.Fprintf(&buf, ", %q: %s", tag, q(value))
			}
		}
	}
	return buf.String()
}

func validMwu(node *Node) bool {
	if node.Cat != "mwu" {
		return false
	}

	if node.NodeList == nil || len(node.NodeList) == 0 {
		return false
	}

	for _, n := range node.NodeList {
		/*
			if i > 0 && node.NodeList[i-1].End != n.Begin {
				return false
			}
		*/
		if n.Rel != "mwp" {
			return false
		}

		if n.Word == "" && len(n.NodeList) == 0 && n.Index != "" {
			n = refnodes[n.index]
		}

		if n.Word == "" {
			return false
		}
	}

	return true
}

func isNP(node *Node) bool {
	if node.Cat == "conj" {
		for _, n := range node.NodeList {
			if isNP(n) {
				return true
			}
		}
		return false
	}

	if node.Cat == "np" {
		return true
	}

	if node.Lcat == "np" && node.Rel != "hd" && node.Rel != "mwp" {
		return true
	}

	if node.Pt == "n" && node.Rel != "hd" {
		return true
	}

	if node.Pt == "vnw" && node.Pdtype == "pron" && node.Rel != "hd" {
		return true
	}

	if node.Cat == "mwu" && (node.Rel == "su" || node.Rel == "obj1" || node.Rel == "obj2" || node.Rel == "app") {
		return true
	}

	return false
}

func isVorfeld(node *Node) bool {
	/*
		%PQ_precedes_head_of_smain%
		and
		not(parent::node[%PQ_precedes_head_of_smain%])
		and(@cat or @pt)
	*/
	if !precedes_head_of_main(node) {
		return false
	}
	for _, n := range node.parents {
		if precedes_head_of_main(n) {
			return false
		}
		break // alleen eerste parent!
	}
	return true
}

func precedes_head_of_main(node *Node) bool {
	/*
	   ancestor::node[@cat="smain"]/node[@rel="hd"]/number(@begin) > node[%PQ_headrel%]/number(@begin)
	   or
	   (  ancestor::node[@cat="smain"]/node[@rel="hd"]/number(@begin) > number(@begin)
	      and
	      not(node[%PQ_headrel%])
	   )
	*/
	begins := make([]int, 0)
	for _, n := range node.parents {
		ancestors_smain_hd_begin(n, &begins)
		break // alleen eerste parent!
	}
	node_headrel_begins := make([]int, 0)
	for _, n := range node.NodeList {
		headrel_begin(n, &node_headrel_begins)
	}
	if len(node_headrel_begins) > 0 {
		for _, b := range begins {
			for _, h := range node_headrel_begins {
				if b > h {
					return true
				}
			}
		}
	} else {
		for _, b := range begins {
			if b > node.Begin {
				return true
			}
		}
	}
	return false
}

func ancestors_smain_hd_begin(node *Node, begins *[]int) {
	if node.Cat == "smain" {
		for _, n := range node.NodeList {
			if n.Rel == "hd" {
				*begins = append(*begins, n.Begin)
			}
		}
	}
	for _, n := range node.parents {
		ancestors_smain_hd_begin(n, begins)
		break // alleen eerste parent!
	}
}

func headrel_begin(node *Node, begins *[]int) {
	/*
		@rel=("hd","cmp","mwp","crd","rhd","whd","nucl","dp")
	*/
	if node.Rel == "hd" ||
		node.Rel == "cmp" ||
		node.Rel == "mwp" ||
		node.Rel == "crd" ||
		node.Rel == "rhd" ||
		node.Rel == "whd" ||
		node.Rel == "nucl" ||
		node.Rel == "dp" {
		*begins = append(*begins, node.Begin)
	}
}

func isDeste(node *Node) bool {
	if node.NodeList == nil {
		return false
	}

	graad := false
	for _, n := range node.NodeList {
		if n.Graad == "comp" {
			graad = true
			break
		}
	}
	if !graad {
		return false
	}

	for _, n := range node.NodeList {
		if n.Lemma == "hoe" || n.Lemma == "deste" {
			return true
		}
		if n.NodeList != nil {
			des := false
			te := false
			for _, nn := range n.NodeList {
				if nn.Lemma == "des" {
					des = true
				} else if nn.Lemma == "te" {
					te = true
				}
				if des && te {
					return true
				}
			}
		}
	}

	return false
}

func escape(s string) string {
	return strings.Replace(fmt.Sprintf("%q", s), "'", "''", -1)
}

func sq(s string) string {
	return strings.Replace(strings.Replace(s, "'", "''", -1), `\`, `\\`, -1)
}

func basename(name string) string {
	corpus := filepath.Base(name)
	n := len(corpus)
	if strings.HasSuffix(corpus, ".dact") {
		return corpus[:n-5]
	}
	if strings.HasSuffix(corpus, ".index") {
		return corpus[:n-6]
	}
	if strings.HasSuffix(corpus, ".data.dz") {
		return corpus[:n-8]
	}
	return corpus
}

func getFiles() {
	for _, filename := range flag.Args() {
		chArch <- filename
		if strings.HasSuffix(filename, ".dact") {
			db, err := dbxml.OpenRead(filename)
			x(err)
			docs, err := db.All()
			x(err)
			for docs.Next() {
				chDoc <- Doc{name: docs.Name(), data: []byte(docs.Content())}
			}
			db.Close()
		} else if strings.HasSuffix(filename, ".index") || strings.HasSuffix(filename, ".data.dz") {
			corpus, err := compactcorpus.Open(filename)
			x(err)
			it, err := corpus.NewRange()
			x(err)
			for it.HasNext() {
				name, data := it.Next()
				chDoc <- Doc{name: name, data: data}
			}
		} else {
			// TODO: .xml .xml.gz
			x(fmt.Errorf("Unknown file type for %s", filename))
		}
	}
	close(chDoc)
}

func featureCount(item, jsn string) {
	var j map[string]interface{}
	jsn = strings.Replace(jsn, `\\`, `\`, -1)
	x(json.Unmarshal([]byte(jsn), &j), jsn)
	for key := range j {
		featureMap[item][key] = featureMap[item][key] + 1
	}
}
