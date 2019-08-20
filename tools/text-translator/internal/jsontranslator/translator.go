package jsontranslator

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
)

// JSONTranslation represents a translation for a locale
type JSONTranslation struct {
	Locale string
	Items  []Translation
}

// Translation represent a translation item for a locale
type Translation struct {
	Locale      string `json:"locale"`
	Key         string `json:"key"`
	Translation string `json:"trans"`
}

// TranslateService translates a text from the sourceLocale to the
// targetLocale
type TranslateService interface {
	T(text string, sourceLocale string, targetLocale string) (string, error)
}

// Read parses a JSON file
func Read(path string) (*JSONTranslation, error) {
	// TODO: possibly use os.Stat to support paths of directories to make translate multiple files easier
	log.Printf("reading %s json file...\n", path)
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error while opening template file %v", path)
	}
	defer file.Close()

	content, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, errors.Wrap(err, "error while reading contents of template file")
	}

	var res []Translation
	err = json.Unmarshal(content, &res)
	if err != nil {
		return nil, errors.Wrap(err, "error while unmarshaling template content")
	}

	if len(res) == 0 {
		return nil, errors.New("file doesn't have any translation items")
	}

	return &JSONTranslation{
		Locale: res[0].Locale,
		Items:  res,
	}, nil
}

// Translate translates items to multiple target locales using the provided
// translator service
func Translate(ts TranslateService, source *JSONTranslation, targetLocales []string) []JSONTranslation {
	res := make([]JSONTranslation, len(targetLocales))
	var wg sync.WaitGroup
	for i := range targetLocales {
		wg.Add(1)
		go func(i int) {
			log.Printf("translating into %v...\n", targetLocales[i])
			res[i] = JSONTranslation{
				Locale: targetLocales[i],
				Items:  make([]Translation, 0, len(source.Items)),
			}
			for _, m := range source.Items {
				output, err := ts.T(m.Translation, m.Locale, targetLocales[i])
				if err != nil {
					log.Printf("coudn't translate %v: %v\n", m.Translation, err)
				}
				res[i].Items = append(res[i].Items, Translation{
					Locale:      targetLocales[i],
					Key:         m.Key,
					Translation: output,
				})
			}
			wg.Done()
		}(i)
	}
	wg.Wait()

	return res
}

// Save saves the translations in the path directory
func Save(dirPath string, name string, translations []JSONTranslation) error {
	// TODO: consider "merge" option to avoid overriding existing files
	for _, t := range translations {
		log.Printf("saving locale %v json file...\n", t.Locale)
		b, err := json.MarshalIndent(t.Items, "", "   ")
		if err != nil {
			return errors.Wrap(err, "error while serializing json file")
		}
		folderPath := filepath.Join(dirPath, t.Locale)
		os.MkdirAll(folderPath, os.ModePerm)
		err = ioutil.WriteFile(filepath.Join(folderPath, name), b, 0644)
		if err != nil {
			return errors.Wrap(err, "error while writing to the json file")
		}
	}

	return nil
}
