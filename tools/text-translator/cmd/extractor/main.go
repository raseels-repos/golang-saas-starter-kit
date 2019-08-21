package main

import (
	"flag"
	"fmt"
	"geeks-accelerator/oss/saas-starter-kit/tools/text-translator/internal/jsontranslator"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const (
	keyWordLengthLimit = 4
)

var (
	inFile = flag.String("i", "", "source .gohtml file to extract text from")
	outDir = flag.String("o", "", "output directory for translations")
	locale = flag.String("l", "en", "locale of input file")

	allowedCharsKeyRegex, _ = regexp.Compile("[^a-zA-Z0-9 ]+")
)

func main() {
	flag.Parse()
	flag.VisitAll(func(f *flag.Flag) {
		if f.Value.String() == "" {
			fmt.Printf("-%s flag is required\n", f.Name)
			os.Exit(1)
		}
	})

	s, err := os.Stat(*inFile)
	if err != nil {
		fmt.Printf("coudn't check if path is a file or directory: %v\n", err)
		os.Exit(1)
	}

	if s.IsDir() {
		filepath.Walk(*inFile, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Printf("error while walking path: %v\n", err)
				os.Exit(1)
			}
			if !info.IsDir() {
				parseFile(path)
			}
			return nil
		})
	} else {
		parseFile(*inFile)
	}

}

func parseFile(path string) {
	log.Printf("reading file %s\n", path)
	b, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("error while reading input file: %v\n", err)
		os.Exit(1)
	}
	content := string(b)
	_, name := filepath.Split(path)
	filenameWithoutExt := strings.TrimRight(name, filepath.Ext(name))

	translationFile := jsontranslator.JSONTranslation{Locale: *locale}

	// Extract title and description
	title, description := extractTitleAndDescription(content)
	if title != "" {
		translationFile.Items = append(translationFile.Items, jsontranslator.Translation{
			Locale:      *locale,
			Key:         fmt.Sprintf("%s-title", filenameWithoutExt),
			Translation: title,
		})
	}
	if description != "" {
		translationFile.Items = append(translationFile.Items, jsontranslator.Translation{
			Locale:      *locale,
			Key:         fmt.Sprintf("%s-description", filenameWithoutExt),
			Translation: description,
		})
	}

	// Extract texts from html tags
	extractedTexts := unique(extract(content, filenameWithoutExt))
	for _, text := range extractedTexts {
		key := makeKey(text, filenameWithoutExt)
		translationFile.Items = append(translationFile.Items, jsontranslator.Translation{
			Locale:      *locale,
			Key:         key,
			Translation: text,
		})
	}

	err = jsontranslator.Save(*outDir, filenameWithoutExt+".json", []jsontranslator.JSONTranslation{translationFile})
	if err != nil {
		fmt.Printf("error while saving the extracted strings: %v\n", err)
		os.Exit(1)
	}
}

func extract(content string, keyPrefix string) []string {
	var res []string
	r := strings.NewReader(content)
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		log.Fatal(err)
	}

	f := func(text string) {
		// Safemode: avoid contents that involve template actions
		if strings.Index(text, "{{") != -1 {
			return
		}
		if len(text) > 0 {
			res = append(res, text)
		}
	}

	// Extract <input> placeholder texts
	doc.Find("input").Union(doc.Find("select")).Each(func(i int, input *goquery.Selection) {
		if p, exists := input.Attr("placeholder"); exists {
			f(p)
		}
	})

	// Extract text from <p>, <a>, ... tags
	simpleTags := []string{"p", "a", "h1", "h2", "h3", "h4", "h5", "h6", "button", "small", "label", "li"}
	n := &goquery.Selection{}
	for _, tag := range simpleTags {
		n = n.Union(doc.Find(tag))
	}
	n.Each(func(i int, n *goquery.Selection) {
		if n.Children().Length() == 0 {
			f(n.Text())
		}
	})

	return res
}

func extractTitleAndDescription(content string) (title, description string) {
	idxStart := strings.Index(content, `{{define "title"}}`)
	if idxStart >= 0 {
		idxEnd := strings.Index(content, `{{end}}`)
		if idxEnd >= 0 {
			title = content[idxStart+18 : idxEnd]
			content = content[idxEnd+7:]
		}
	}

	idxStart = strings.Index(content, `{{define "description"}}`)
	if idxStart >= 0 {
		idxEnd := strings.Index(content, `{{end}}`)
		if idxEnd >= 0 {
			description = content[idxStart+24 : idxEnd]
		}
	}
	return
}

func makeKey(text, prefix string) string {
	text = strings.TrimSpace(text)
	key := allowedCharsKeyRegex.ReplaceAllString(text, "")
	key = strings.ToLower(key)

	split := strings.SplitN(key, " ", keyWordLengthLimit+1)
	limit := len(split)
	if limit > keyWordLengthLimit {
		limit = keyWordLengthLimit
	}
	key = strings.Join(split[:limit], "-")

	key = fmt.Sprintf("%s-%s", prefix, key)

	return key
}

func unique(lst []string) []string {
	keys := make(map[string]struct{})
	list := []string{}
	for _, s := range lst {
		if _, exist := keys[s]; !exist {
			keys[s] = struct{}{}
			list = append(list, s)
		}
	}
	return list
}
