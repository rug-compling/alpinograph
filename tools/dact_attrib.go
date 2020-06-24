package main

import (
	"github.com/pebbe/dbxml"
	"github.com/pebbe/util"
	"github.com/rug-compling/alpinods"

	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type what int

const (
	insert = what(iota)
	delete
)

type action struct {
	Xpath string
	What  what
	Name  string
	Value string
}

var (
	macrofile = flag.String("m", "", "macrofile")
	macroKY   = regexp.MustCompile(`%[a-zA-Z][_a-zA-Z0-9]*%`)
	macroRE   = regexp.MustCompile(`([a-zA-Z][_a-zA-Z0-9]*)\s*=\s*"""((?s:.*?))"""`)
	x         = util.CheckErr

	reDate     = regexp.MustCompile("^[1-9][0-9][0-9][0-9]-[01][0-9]-[0-3][0-9]$")
	reDateTime = regexp.MustCompile("^[1-9][0-9][0-9][0-9]-[01][0-9]-[0-3][0-9] [ 0-2]?[0-9]:[0-5][0-9](:[0-5][0-9])?$")
)

func usage() {

	fmt.Printf(`
Usage: %s [-m macrofile] infile.dact outfile.dact xpath action [xpath action ...]

   xpath:  xpath expression matching some node in Alpino

   action: add data to node, i.e.:

     node:NAME
     node:NAME:TYPE:VALUE
     rel:NAME
     rel:NAME:TYPE:VALUE

       node:NAME is the same as node:NAME:bool:true
       rel:NAME is the same as rel:NAME:bool:true

       valid TYPE: text, int, float, bool, date, datetime

         bool:     true, false
         date:     2020-12-31
         datetime: 2020-12-31 23:59
         datetime: 2020-12-31 23:59:59

   -- or --

   action: delete data from node, i.e.:

     delete:node:NAME:TYPE
     delete:rel:NAME:TYPE

`, os.Args[0])

}

func main() {
	flag.Usage = usage
	flag.Parse()
	narg := flag.NArg()
	if narg < 4 || narg%2 != 0 {
		usage()
		return
	}

	rules := map[string]string{}
	if *macrofile != "" {
		rules = loadMacros()
	}

	nxpath := (narg - 2) / 2

	infile := flag.Arg(0)
	outfile := flag.Arg(1)

	actions := make([]action, nxpath)
	for i := 0; i < nxpath; i++ {
		aa := strings.SplitN(flag.Arg(3+2*i), ":", 4)
		var name, typ, value string
		var wh what
		if len(aa) == 2 {
			aa = append(aa, "bool", "true")
		}
		if len(aa) != 4 {
			x(fmt.Errorf("[%d] missing fields in %q", i+1, flag.Arg(3+2*i)))
		}
		if aa[0] == "delete" {
			wh = delete
			aa = aa[1:]
		} else {
			wh = insert
		}
		if aa[0] != "node" && aa[0] != "rel" {
			x(fmt.Errorf("[%d] invalid target %q", i+1, aa[0]))
		}
		aa[1] = strings.TrimSpace(aa[1])
		if len(aa[1]) == 0 {
			x(fmt.Errorf("[%d] missing name in %q", i+1, aa))
		}
		name = "ag:" + aa[0] + ":" + aa[2] + ":" + aa[1]
		if wh == insert {
			typ = aa[2]
			value = strings.TrimSpace(aa[3])
			switch typ {
			case "text":
				if len(value) == 0 {
					x(fmt.Errorf("[%d] missing value", i+1))
				}
			case "int":
				_, err := strconv.Atoi(value)
				if err != nil {
					x(fmt.Errorf("[%d] %v", i+1, err))
				}
			case "float":
				_, err := strconv.ParseFloat(value, 64)
				if err != nil {
					x(fmt.Errorf("[%d] %v", i+1, err))
				}
			case "date":
				if !reDate.MatchString(value) {
					x(fmt.Errorf("[%d] invalid date value %q", i+1, value))
				}
			case "datetime":
				if !reDateTime.MatchString(value) {
					x(fmt.Errorf("[%d] invalid datetime value %q", i+1, value))
				}
			case "bool":
				if value != "true" && value != "false" {
					x(fmt.Errorf("[%d] invalid bool value %q", i+1, value))
				}
			default:
				x(fmt.Errorf("[%d] invalid type %q", i+1, typ))
			}
		}
		actions[i] = action{
			Xpath: flag.Arg(2 + 2*i),
			What:  wh,
			Name:  name,
			Value: value,
		}
	}

	db1, err := dbxml.OpenRead(infile)
	x(err)
	matches := make(map[string][]map[int]bool)
	for xp, action := range actions {
		query := action.Xpath
		if strings.Contains(query, "%") {
			query = macroKY.ReplaceAllStringFunc(query, func(s string) string {
				return rules[s[1:len(s)-1]]
			})
		}
		docs, err := db1.Query(query)
		x(err)
		n := 0
		for docs.Next() {
			n++
			var node alpinods.Node
			x(xml.Unmarshal([]byte(docs.Match()), &node))
			file := docs.Name()
			fmt.Printf("  MATCH %d: %s        \r", xp+1, file)
			if _, ok := matches[file]; !ok {
				matches[file] = make([]map[int]bool, nxpath)
				for i := 0; i < nxpath; i++ {
					matches[file][i] = make(map[int]bool)
				}
			}
			matches[file][xp][node.ID] = true
		}
		x(docs.Error())
		fmt.Printf("[%d] %d matches               \n", xp+1, n)
	}
	db1.Close()

	db1, err = dbxml.OpenRead(infile)
	x(err)
	defer db1.Close()
	db2, err := dbxml.OpenReadWrite(outfile)
	x(err)
	defer db2.Close()

	docs, err := db1.All()
	x(err)
	//count := 0
	for docs.Next() {
		file := docs.Name()
		data := docs.Content()
		fmt.Printf("  WRITE: %s        \r", file)
		if xpaths, ok := matches[file]; ok {
			/*
				count++
				fp, err := os.Create(fmt.Sprintf("tmp-%04d-in.xml", count))
				x(err)
				fp.WriteString(data)
				fp.Close()
			*/
			var alpino alpinods.AlpinoDS
			x(xml.Unmarshal([]byte(data), &alpino))
			for n, action := range actions {
				for id := range xpaths[n] {
					node := locate(alpino.Node, id)
					if action.What == delete {
						if node.Data == nil {
							continue
						}
						for i := 0; i < len(node.Data); i++ {
							if node.Data[i].Name == action.Name {
								node.Data = append(node.Data[:i], node.Data[i+1:]...)
								i--
							}
						}
						if len(node.Data) == 0 {
							node.Data = nil
						}
					} else {
						attr := alpinods.Data{
							Name: action.Name,
							Data: action.Value,
						}
						if node.Data == nil {
							node.Data = make([]*alpinods.Data, 0)
						}
						node.Data = append(node.Data, &attr)
					}
				}
			}
			alpino.Version = "1.11"
			data = alpino.String()
		}
		db2.PutXml(file, data, false)
	}
	x(docs.Error())
}

func loadMacros() map[string]string {
	b, err := ioutil.ReadFile(*macrofile)
	x(err, "Reading macrofile")
	macros := make(map[string]string)
	for _, set := range macroRE.FindAllStringSubmatch(string(b), -1) {
		macros[set[1]] = set[2]
	}

	// macro's die macro's bevatten uitpakken
	for key := range macros {
		for {
			rule := macroKY.ReplaceAllStringFunc(macros[key], func(s string) string {
				return macros[s[1:len(s)-1]]
			})
			if rule == macros[key] {
				break
			}
			if len(rule) > 100000 {
				macros[key] = "RECURSIONLIMIT"
				break
			}
			macros[key] = rule
		}
	}

	return macros
}

func locate(node *alpinods.Node, id int) *alpinods.Node {
	if node.ID == id {
		return node
	}
	if node.Node != nil {
		for _, n := range node.Node {
			if n2 := locate(n, id); n2 != nil {
				return n2
			}
		}
	}
	return nil
}
