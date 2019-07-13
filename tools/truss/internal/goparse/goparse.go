package goparse

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"strings"
	"unicode"

	"github.com/pkg/errors"
)

var (
	errGoParseType               = errors.New("Unable to determine type for line")
	errGoTypeMissingCodeTemplate = errors.New("No code defined for type")
	errGoObjectNotExist          = errors.New("GoObject does not exist")
)

// ParseFile reads a go code file and parses into a easily transformable set of objects.
func ParseFile(log *log.Logger, localPath string) (*GoDocument, error) {

	// Read the code file.
	src, err := ioutil.ReadFile(localPath)
	if err != nil {
		err = errors.WithMessagef(err, "Failed to read file %s", localPath)
		return nil, err
	}

	// Format the code file source to ensure parse works.
	dat, err := format.Source(src)
	if err != nil {
		err = errors.WithMessagef(err, "Failed to format source for file %s", localPath)
		log.Printf("ParseFile : %v\n%v", err, string(src))
		return nil, err
	}

	// Loop of the formatted source code and generate a list of code lines.
	lines := []string{}
	r := bytes.NewReader(dat)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		err = errors.WithMessagef(err, "Failed read formatted source code for file %s", localPath)
		return nil, err
	}

	// Parse the code lines into a set of objects.
	objs, err := ParseLines(lines, 0)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// Append the resulting objects to the document.
	doc := &GoDocument{}
	for _, obj := range objs.List() {
		if obj.Type == GoObjectType_Package {
			doc.Package = obj.Name
		}
		doc.AddObject(obj)
	}

	return doc, nil
}

// ParseLines takes the list of formatted code lines and returns the GoObjects.
func ParseLines(lines []string, depth int) (objs *GoObjects, err error) {
	objs = &GoObjects{
		list: []*GoObject{},
	}

	var (
		multiLine    bool
		multiComment bool
		muiliVar     bool
	)
	curDepth := -1
	objLines := []string{}

	for idx, l := range lines {
		ls := strings.TrimSpace(l)

		ld := lineDepth(l)

		//fmt.Println("l", l)
		//fmt.Println("> Depth", ld, "???", depth)

		if ld == depth {
			if strings.HasPrefix(ls, "/*") {
				multiLine = true
				multiComment = true
			} else if strings.HasSuffix(ls, "(") ||
				strings.HasSuffix(ls, "{") {

				if !multiLine {
					multiLine = true
				}
			} else if strings.Contains(ls, "`") {
				if !multiLine && strings.Count(ls, "`")%2 != 0 {
					if muiliVar {
						muiliVar = false
					} else {
						muiliVar = true
					}
				}
			}

			//fmt.Println("> multiLine", multiLine)
			//fmt.Println("> multiComment", multiComment)
			//fmt.Println("> muiliVar", muiliVar)

			objLines = append(objLines, l)

			if multiComment {
				if strings.HasSuffix(ls, "*/") {
					multiComment = false
					multiLine = false
				}
			} else {
				if strings.HasPrefix(ls, ")") ||
					strings.HasPrefix(ls, "}") {
					multiLine = false
				}
			}

			if !multiLine && !muiliVar {
				for eidx := idx + 1; eidx < len(lines); eidx++ {
					if el := lines[eidx]; strings.TrimSpace(el) == "" {
						objLines = append(objLines, el)
					} else {
						break
					}
				}

				//fmt.Println(" > objLines", objLines)

				obj, err := ParseGoObject(objLines, depth)
				if err != nil {
					log.Println(err)
					return objs, err
				}
				err = objs.Add(obj)
				if err != nil {
					log.Println(err)
					return objs, err
				}

				objLines = []string{}
			}

		} else if (multiLine && ld >= curDepth && ld >= depth && len(objLines) > 0) || muiliVar {
			objLines = append(objLines, l)

			if strings.Contains(ls, "`") {
				if !multiLine && strings.Count(ls, "`")%2 != 0 {
					if muiliVar {
						muiliVar = false
					} else {
						muiliVar = true
					}
				}
			}
		}
	}

	for _, obj := range objs.List() {
		children, err := ParseLines(obj.subLines, depth+1)
		if err != nil {
			log.Println(err)
			return objs, err
		}
		for _, child := range children.List() {
			obj.Objects().Add(child)
		}
	}

	return objs, nil
}

// ParseGoObject generates a GoObjected for the given code lines.
func ParseGoObject(lines []string, depth int) (obj *GoObject, err error) {

	// If there are no lines, return a line break.
	if len(lines) == 0 {
		return &GoEmptyLine, nil
	}

	firstLine := lines[0]
	firstStrip := strings.TrimSpace(firstLine)

	if len(firstStrip) == 0 {
		return &GoEmptyLine, nil
	}

	obj = &GoObject{
		goObjects: &GoObjects{
			list: []*GoObject{},
		},
	}

	if strings.HasPrefix(firstStrip, "var") {
		obj.Type = GoObjectType_Var

		if !strings.HasSuffix(firstStrip, "(") {
			if strings.HasPrefix(firstStrip, "var ") {
				firstStrip = strings.TrimSpace(strings.Replace(firstStrip, "var ", "", 1))
			}
			obj.Name = strings.Split(firstStrip, " ")[0]
		}
	} else if strings.HasPrefix(firstStrip, "const") {
		obj.Type = GoObjectType_Const

		if !strings.HasSuffix(firstStrip, "(") {
			if strings.HasPrefix(firstStrip, "const ") {
				firstStrip = strings.TrimSpace(strings.Replace(firstStrip, "const ", "", 1))
			}
			obj.Name = strings.Split(firstStrip, " ")[0]
		}
	} else if strings.HasPrefix(firstStrip, "func") {
		obj.Type = GoObjectType_Func

		if strings.HasPrefix(firstStrip, "func (") {
			funcLine := strings.TrimLeft(strings.TrimSpace(strings.Replace(firstStrip, "func ", "", 1)), "(")

			var structName string
			pts := strings.Split(strings.Split(funcLine, ")")[0], " ")
			for i := len(pts) - 1; i >= 0; i-- {
				ptVal := strings.TrimSpace(pts[i])
				if ptVal != "" {
					structName = ptVal
					break
				}
			}

			var funcName string
			pts = strings.Split(strings.Split(funcLine, "(")[0], " ")
			for i := len(pts) - 1; i >= 0; i-- {
				ptVal := strings.TrimSpace(pts[i])
				if ptVal != "" {
					funcName = ptVal
					break
				}
			}

			obj.Name = fmt.Sprintf("%s.%s", structName, funcName)
		} else {
			obj.Name = strings.Replace(firstStrip, "func ", "", 1)
			obj.Name = strings.Split(obj.Name, "(")[0]
		}
	} else if strings.HasSuffix(firstStrip, "struct {") || strings.HasSuffix(firstStrip, "struct{") {
		obj.Type = GoObjectType_Struct

		if strings.HasPrefix(firstStrip, "type ") {
			firstStrip = strings.TrimSpace(strings.Replace(firstStrip, "type ", "", 1))
		}
		obj.Name = strings.Split(firstStrip, " ")[0]
	} else if strings.HasPrefix(firstStrip, "type") {
		obj.Type = GoObjectType_Type
		firstStrip = strings.TrimSpace(strings.Replace(firstStrip, "type ", "", 1))
		obj.Name = strings.Split(firstStrip, " ")[0]
	} else if strings.HasPrefix(firstStrip, "package") {
		obj.Name = strings.TrimSpace(strings.Replace(firstStrip, "package ", "", 1))

		obj.Type = GoObjectType_Package
	} else if strings.HasPrefix(firstStrip, "import") {
		obj.Type = GoObjectType_Import
	} else if strings.HasPrefix(firstStrip, "//") {
		obj.Type = GoObjectType_Comment
	} else if strings.HasPrefix(firstStrip, "/*") {
		obj.Type = GoObjectType_Comment
	} else {
		if depth > 0 {
			obj.Type = GoObjectType_Line
		} else {
			err = errors.WithStack(errGoParseType)
			return obj, err
		}
	}

	var (
		hasSub        bool
		muiliVarStart bool
		muiliVarSub   bool
		muiliVarEnd   bool
	)
	for _, l := range lines {
		ld := lineDepth(l)
		if (ld == depth && !muiliVarSub) || muiliVarStart || muiliVarEnd {
			if hasSub && !muiliVarStart {
				if strings.TrimSpace(l) != "" {
					obj.endLines = append(obj.endLines, l)
				}

				if strings.Count(l, "`")%2 != 0 {
					if muiliVarEnd {
						muiliVarEnd = false
					} else {
						muiliVarEnd = true
					}
				}
			} else {
				obj.startLines = append(obj.startLines, l)

				if strings.Count(l, "`")%2 != 0 {
					if muiliVarStart {
						muiliVarStart = false
					} else {
						muiliVarStart = true
					}
				}
			}
		} else if ld > depth || muiliVarSub {
			obj.subLines = append(obj.subLines, l)
			hasSub = true

			if strings.Count(l, "`")%2 != 0 {
				if muiliVarSub {
					muiliVarSub = false
				} else {
					muiliVarSub = true
				}
			}
		}
	}

	// add trailing linebreak
	if len(obj.endLines) > 0 {
		obj.endLines = append(obj.endLines, "")
	}

	return obj, err
}

// lineDepth returns the number of spaces for the given code line
// used to determine the code level for nesting objects.
func lineDepth(l string) int {
	depth := len(l) - len(strings.TrimLeftFunc(l, unicode.IsSpace))

	ls := strings.TrimSpace(l)
	if strings.HasPrefix(ls, "}") && strings.Contains(ls, " else ") {
		depth = depth + 1
	} else if strings.HasPrefix(ls, "case ") {
		depth = depth + 1
	}
	return depth
}
