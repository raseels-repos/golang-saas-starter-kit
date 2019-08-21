package main

import (
	"flag"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/tools/text-translator/aws"
	"geeks-accelerator/oss/saas-starter-kit/tools/text-translator/internal/jsontranslator"
	"os"
	"path/filepath"
	"strings"
)

var (
	inFile           = flag.String("i", "", "source file to translate")
	outDir           = flag.String("o", "", "output file to translate")
	csvTargetLocales = flag.String("t", "", "comma separated list of target locales")
)

func main() {
	flag.Parse()
	flag.VisitAll(func(f *flag.Flag) {
		if f.Value.String() == "" {
			fmt.Printf("-%s flag is required\n", f.Name)
			os.Exit(1)
		}
	})

	t, err := aws.New()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	sourceTrans, err := jsontranslator.Read(*inFile)
	if err != nil {
		fmt.Printf("error while reading json file: %v", err)
		os.Exit(1)
	}

	targetLocales := strings.Split(*csvTargetLocales, ",")
	targetTranslations := jsontranslator.Translate(t, sourceTrans, targetLocales)

	_, name := filepath.Split(*inFile)
	err = jsontranslator.Save(*outDir, name, targetTranslations)
	if err != nil {
		fmt.Printf("error while saving translations: %v", err)
		os.Exit(1)
	}
}
