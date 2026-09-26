// parity: prints the same output as tests/parity.js so the Go port can be diffed against engine.js.
//
//	parity keys  <words.tsv>                          -> word<TAB>keys<TAB>rules
//	parity suggest <lexicon.tsv> <en-words.txt> < inputs -> input<TAB>suggestions<TAB>with context<TAB>completions<TAB>keep-English
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"marathityping/engine"
)

const userDict = "# test\nek = एक समाज\nprem = प्रेमकुमार\nसिंदखेडराजा\nIITD\tआयआयटी दिल्ली\n bad line =\n= x\n"

func main() {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	switch os.Args[1] {
	case "keys":
		f, _ := os.Open(os.Args[2])
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			word := strings.SplitN(sc.Text(), "\t", 2)[0]
			fmt.Fprintf(w, "%s\t%s\t%s\n", word, strings.Join(engine.WordKeys(word), " "), engine.RulesToDev(strings.Join(engine.DevToRomanVariants(word), "")))
		}
	case "suggest":
		e := engine.New()
		f, _ := os.Open(os.Args[2])
		if err := e.Load(f); err != nil {
			panic(err)
		}
		g, _ := os.Open(os.Args[3])
		e.LoadEnglish(g)
		n := e.SetUserDict(userDict)
		fmt.Fprintf(w, "userdict\t%d\n", n)
		e.Learn("kal", "काळ")
		e.Learn("constructor", "X")
		e.LearnContext("मी", "आलो")
		e.LearnContext("काल", "रात्री")
		e.LearnContext("काल", "रात्री")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			in := sc.Text()
			s := e.Suggest(in, 7, "")
			ctx := e.Suggest(in, 7, "काल")
			comp := e.Complete(in, 2, s)
			e.KeepEnglish = true
			ke := e.Suggest(in, 7, "मी")
			e.KeepEnglish = false
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", in, strings.Join(s, "|"), strings.Join(ctx, "|"), strings.Join(comp, "|"), strings.Join(ke, "|"))
		}
	}
}
