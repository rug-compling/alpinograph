package main

import (
	"github.com/pebbe/util"

	"bufio"
	"fmt"
	"os"
	"strings"
)

var (
	x = util.CheckErr
)

func main() {
	fmt.Print("package main\n\nvar corpora = map[string]string{\n")
	fp, err := os.Open("../corpora.txt")
	x(err)
	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == ':' {
			continue
		}
		a := strings.Fields(line)
		fmt.Printf("\t%q: %q,\n", a[0], strings.Join(a[2:], " "))
	}
	fp.Close()
	fmt.Println("}")
}
