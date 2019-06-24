package dbtable2crud

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"geeks-accelerator/oss/saas-starter-kit/example-project/tools/truss/internal/goparse"
	"github.com/dustin/go-humanize/english"
	"github.com/fatih/camelcase"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
)

// loadTemplateObjects executes a template file based on the given model struct and
// returns the parsed go objects.
func loadTemplateObjects(log *log.Logger, model *modelDef, templateDir, filename string, tmptData map[string]interface{}) ([]*goparse.GoObject, error) {

	// Data used to execute all the of defined code sections in the template file.
	if tmptData == nil {
		tmptData = make(map[string]interface{})
	}
	tmptData["Model"] = model

	// geeks-accelerator/oss/saas-starter-kit/example-project

	// Read the template file from the local file system.
	tempFilePath := filepath.Join(templateDir, filename)
	dat, err := ioutil.ReadFile(tempFilePath)
	if err != nil {
		err = errors.WithMessagef(err, "Failed to read template file %s",  tempFilePath)
		return nil, err
	}

	// New template with custom functions.
	baseTmpl := template.New("base")
	baseTmpl.Funcs(template.FuncMap{
		"Concat": func(vals ...string) string {
			return strings.Join(vals, "")
		},
		"JoinStrings": func(vals []string, sep string ) string {
			return strings.Join(vals, sep)
		},
		"PrefixAndJoinStrings": func(vals []string, pre, sep string ) string {
			l := []string{}
			for _, v := range vals {
				l = append(l, pre + v)
			}
			return strings.Join(l, sep)
		},
		"FmtAndJoinStrings": func(vals []string, fmtStr, sep string ) string {
			l := []string{}
			for _, v := range vals {
				l = append(l, fmt.Sprintf(fmtStr, v))
			}
			return strings.Join(l, sep)
		},
		"FormatCamel": func(name string) string {
			return FormatCamel(name)
		},
		"FormatCamelTitle": func(name string) string {
			return FormatCamelTitle(name)
		},
		"FormatCamelLower": func(name string) string {
			if name == "ID" {
				return "id"
			}
			return FormatCamelLower(name)
		} ,
		"FormatCamelLowerTitle": func(name string) string {
			return FormatCamelLowerTitle(name)
		} ,
		"FormatCamelPluralTitle": func(name string) string {
			return FormatCamelPluralTitle(name)
		} ,
		"FormatCamelPluralTitleLower": func(name string) string {
			return FormatCamelPluralTitleLower(name)
		} ,
		"FormatCamelPluralCamel": func(name string) string {
			return FormatCamelPluralCamel(name)
		} ,
		"FormatCamelPluralLower": func(name string) string {
			return FormatCamelPluralLower(name)
		} ,
		"FormatCamelPluralUnderscore": func(name string) string {
			return FormatCamelPluralUnderscore(name)
		} ,
		"FormatCamelPluralLowerUnderscore": func(name string) string {
			return FormatCamelPluralLowerUnderscore(name)
		} ,
		"FormatCamelUnderscore": func(name string) string {
			return FormatCamelUnderscore(name)
		} ,
		"FormatCamelLowerUnderscore": func(name string) string {
			return FormatCamelLowerUnderscore(name)
		} ,
		"FieldTagHasOption": func(f modelField, tagName, optName string ) bool {
			if f.Tags == nil {
				return false
			}
			ft, err := f.Tags.Get(tagName)
			if ft == nil || err != nil {
				return false
			}
			if ft.Name == optName || ft.HasOption(optName) {
				return true
			}
			return false
		},
		"FieldTag": func(f modelField, tagName string) string {
			if f.Tags == nil {
				return ""
			}
			ft, err := f.Tags.Get(tagName)
			if ft == nil || err != nil {
				return ""
			}
			return ft.String()
		},
		"FieldTagReplaceOrPrepend": func(f modelField, tagName, oldVal, newVal string) string {
			if f.Tags == nil {
				return ""
			}
			ft, err := f.Tags.Get(tagName)
			if ft == nil || err != nil {
				return ""
			}

			if ft.Name == oldVal || ft.Name == newVal {
				ft.Name = newVal
			} else if ft.HasOption(oldVal) {
				for idx, val := range ft.Options {
					if val == oldVal {
						ft.Options[idx] = newVal
					}
				}
			} else if !ft.HasOption(newVal) {
				if ft.Name == ""{
					ft.Name = newVal
				} else {
					ft.Options = append(ft.Options, newVal)
				}
			}

			return ft.String()
		},
		"StringListHasValue": func(list []string, val string) bool {
			for _, v := range list {
				if v == val {
					return true
				}
			}
			return false
		},
	})

	// Load the template file using the text/template package.
	tmpl, err := baseTmpl.Parse(string(dat))
	if err != nil {
		err = errors.WithMessagef(err, "Failed to parse template file %s",  tempFilePath)
		log.Printf("loadTemplateObjects : %v\n%v", err, string(dat))
		return nil, err
	}

	// Generate a list of template names defined in the template file.
	tmplNames := []string{}
	for _, defTmpl := range tmpl.Templates() {
		tmplNames = append(tmplNames, defTmpl.Name())
	}

	// Stupid hack to return template names the in order they are defined in the file.
	tmplNames, err = templateFileOrderedNames(tempFilePath, tmplNames)
	if err != nil {
		return nil, err
	}

	// Loop over all the defined templates, execute using the defined data, parse the
	// formatted code and append the parsed go objects to the result list.
	var resp []*goparse.GoObject
	for _, tmplName := range tmplNames {
		// Executed the defined template with the given data.
		var tpl bytes.Buffer
		if err := tmpl.Lookup(tmplName).Execute(&tpl, tmptData); err != nil {
			err = errors.WithMessagef(err, "Failed to execute %s from template file %s",  tmplName, tempFilePath)
			return resp, err
		}

		// Format the source code to ensure its valid and code to parsed consistently.
		codeBytes, err := format.Source(tpl.Bytes())
		if err != nil {
			err = errors.WithMessagef(err, "Failed to format source for %s in template file %s",  tmplName, filename)

			dl := []string{}
			for idx, l := range strings.Split(tpl.String(), "\n") {
				dl = append(dl, fmt.Sprintf("%d -> ", idx) + l)
			}

			log.Printf("loadTemplateObjects : %v\n%v", err, strings.Join(dl, "\n"))
			return resp, err
		}

		// Remove extra white space from the code.
		codeStr := strings.TrimSpace(string(codeBytes))

		// Split the code into a list of strings.
		codeLines := strings.Split(codeStr, "\n")

		// Parse the code lines into a set of objects.
		objs, err := goparse.ParseLines(codeLines, 0)
		if err != nil {
			err = errors.WithMessagef(err, "Failed to parse %s in template file %s",  tmplName, filename)
			log.Printf("loadTemplateObjects : %v\n%v", err, codeStr)
			return resp, err
		}

		// Append the parsed objects to the return result list.
		for _, obj := range objs.List() {
			if obj.Name == "" && obj.Type != goparse.GoObjectType_Import && obj.Type != goparse.GoObjectType_Var && obj.Type != goparse.GoObjectType_Const && obj.Type != goparse.GoObjectType_Comment && obj.Type != goparse.GoObjectType_LineBreak {
				// All objects should have a name except for multiline var/const declarations and comments.
				err = errors.Errorf("Failed to parse name with type %s from lines: %v", obj.Type, obj.Lines())
				return resp, err
			} else if string(obj.Type) == "" {
				err = errors.Errorf("Failed to parse type for %s from lines: %v", obj.Name, obj.Lines())
				return resp, err
			}

			resp = append(resp, obj)
		}
	}

	return resp, nil
}

// FormatCamel formats Valdez mountain to ValdezMountain
func FormatCamel(name string) string {
	return strcase.ToCamel(name)
}

// FormatCamelLower formats ValdezMountain to valdezmountain
func FormatCamelLower(name string) string {
	return strcase.ToLowerCamel(FormatCamel(name))
}

// FormatCamelTitle formats ValdezMountain to Valdez Mountain
func FormatCamelTitle(name string) string {
	return strings.Join(camelcase.Split(name), " ")
}

// FormatCamelLowerTitle formats ValdezMountain to valdez mountain
func FormatCamelLowerTitle(name string) string {
	return strings.ToLower(FormatCamelTitle(name))
}

// FormatCamelPluralTitle formats ValdezMountain to Valdez Mountains
func FormatCamelPluralTitle(name string) string {
	pts := camelcase.Split(name)
	lastIdx := len(pts) - 1
	pts[lastIdx] = english.PluralWord(2, pts[lastIdx], "")
	return strings.Join(pts, " ")
}

// FormatCamelPluralTitleLower formats ValdezMountain to valdez mountains
func FormatCamelPluralTitleLower(name string) string {
	return strings.ToLower(FormatCamelPluralTitle(name))
}

// FormatCamelPluralCamel formats ValdezMountain to ValdezMountains
func FormatCamelPluralCamel(name string) string {
	return strcase.ToCamel(FormatCamelPluralTitle(name))
}

// FormatCamelPluralLower formats ValdezMountain to valdezmountains
func FormatCamelPluralLower(name string) string {
	return strcase.ToLowerCamel(FormatCamelPluralTitle(name))
}

// FormatCamelPluralUnderscore formats ValdezMountain to Valdez_Mountains
func FormatCamelPluralUnderscore(name string) string {
	return strings.Replace(FormatCamelPluralTitle(name), " ", "_", -1)
}

// FormatCamelPluralLowerUnderscore formats ValdezMountain to valdez_mountains
func FormatCamelPluralLowerUnderscore(name string) string {
	return strings.ToLower(FormatCamelPluralUnderscore(name))
}

// FormatCamelUnderscore formats ValdezMountain to Valdez_Mountain
func FormatCamelUnderscore(name string) string {
	return strings.Replace(FormatCamelTitle(name), " ", "_", -1)
}

// FormatCamelLowerUnderscore formats ValdezMountain to valdez_mountain
func FormatCamelLowerUnderscore(name string) string {
	return strings.ToLower(FormatCamelUnderscore(name))
}

// templateFileOrderedNames returns the template names the in order they are defined in the file.
func templateFileOrderedNames(localPath string, names []string) (resp []string, err error) {
	file, err := os.Open(localPath)
	if err != nil {
		return resp, errors.WithStack(err)
	}
	defer file.Close()

	idxList := []int{}
	idxNames := make(map[int]string)

	idx := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if !strings.HasPrefix(scanner.Text(), "{{") ||  !strings.Contains(scanner.Text(), "define ") {
			continue
		}

		for _, name := range names {
			if strings.Contains(scanner.Text(), "\""+name+"\"") {
				idxList = append(idxList, idx)
				idxNames[idx] = name
				break
			}
		}

		idx = idx + 1
	}

	if err := scanner.Err(); err != nil {
		return resp, errors.WithStack(err)
	}

	sort.Ints(idxList)

	for _, idx := range idxList {
		resp = append(resp, idxNames[idx])
	}

	return resp, nil
}
