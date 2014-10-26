package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var flagPackage = flag.String("pkg", "res", "package name")

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
	}

	for _, name := range flag.Args() {
		err := Write(name)
		if err != nil {
			log.Println(name, err)
		}
	}
}

func Write(name string) error {
	b, err := ioutil.ReadFile(name)
	if err != nil {
		return err
	}

	f, err := os.Create(name + ".go")
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "package %s\n\nconst %s = %q\n", *flagPackage, Name(name), b)
	return err
}

func Name(orig string) string {
	var name []rune

	pieces := strings.FieldsFunc(filepath.Base(orig), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	for _, piece := range pieces {
		p := []rune(piece)
		name = append(name, unicode.ToUpper(p[0]))
		name = append(name, p[1:]...)
	}

	if !unicode.IsLetter(name[0]) {
		name = append([]rune{'X'}, name...)
	}

	return string(name)
}
