package goparse

import (
	"fmt"
	"go/format"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
)

// GoDocument defines a single go code file.
type GoDocument struct {
	*GoObjects
	Package string
	imports GoImports
}

// GoImport defines a single import line with optional alias.
type GoImport struct {
	Name  string
	Alias string
}

// GoImports holds a list of import lines.
type GoImports []GoImport

// NewGoDocument creates a new GoDocument with the package line set.
func NewGoDocument(packageName string) (doc *GoDocument, err error) {
	doc = &GoDocument{
		GoObjects: &GoObjects{
			list: []*GoObject{},
		},
	}
	err = doc.SetPackage(packageName)
	return doc, err
}

// Objects returns a list of root GoObject.
func (doc *GoDocument) Objects() *GoObjects {
	if doc.GoObjects == nil {
		doc.GoObjects = &GoObjects{
			list: []*GoObject{},
		}
	}

	return doc.GoObjects
}

// NewObjectPackage returns a new GoObject with a single package definition line.
func NewObjectPackage(packageName string) *GoObject {
	lines := []string{
		fmt.Sprintf("package %s", packageName),
		"",
	}

	obj, _ := ParseGoObject(lines, 0)

	return obj
}

// SetPackage appends sets the package line for the code file.
func (doc *GoDocument) SetPackage(packageName string) error {

	var existing *GoObject
	for _, obj := range doc.Objects().List() {
		if obj.Type == GoObjectType_Package {
			existing = obj
			break
		}
	}

	new := NewObjectPackage(packageName)

	var err error
	if existing != nil {
		err = doc.Objects().Replace(existing, new)
	} else if len(doc.Objects().List()) > 0 {

		// Insert after any existing comments or line breaks.
		var insertPos int
		//for idx, obj := range doc.Objects().List() {
		//	switch obj.Type {
		//	case GoObjectType_Comment, GoObjectType_LineBreak:
		//		insertPos = idx
		//	default:
		//		break
		//	}
		//}

		err = doc.Objects().Insert(insertPos, new)
	} else {
		err = doc.Objects().Add(new)
	}

	return err
}

// AddObject appends a new GoObject to the doc root object list.
func (doc *GoDocument) AddObject(newObj *GoObject) error {
	return doc.Objects().Add(newObj)
}

// InsertObject inserts a new GoObject at the desired position to the doc root object list.
func (doc *GoDocument) InsertObject(pos int, newObj *GoObject) error {
	return doc.Objects().Insert(pos, newObj)
}

// Imports returns the GoDocument imports.
func (doc *GoDocument) Imports() (GoImports, error) {
	// If the doc imports are empty, try to load them from the root objects.
	if len(doc.imports) == 0 {
		for _, obj := range doc.Objects().List() {
			if obj.Type != GoObjectType_Import {
				continue
			}

			res, err := ParseImportObject(obj)
			if err != nil {
				return doc.imports, err
			}

			// Combine all the imports into a single definition.
			for _, n := range res {
				doc.imports = append(doc.imports, n)
			}
		}
	}

	return doc.imports, nil
}

// Lines returns all the code lines.
func (doc *GoDocument) Lines() []string {
	l := []string{}

	for _, ol := range doc.Objects().Lines() {
		l = append(l, ol)
	}
	return l
}

// String returns a single value for all the code lines.
func (doc *GoDocument) String() string {
	return strings.Join(doc.Lines(), "\n")
}

// Print writes all the code lines to standard out.
func (doc *GoDocument) Print() {
	for _, l := range doc.Lines() {
		fmt.Println(l)
	}
}

// Save renders all the code lines for the document, formats the code
// and then saves it to the supplied file path.
func (doc *GoDocument) Save(localpath string) error {
	res, err := format.Source([]byte(doc.String()))
	if err != nil {
		err = errors.WithMessage(err, "Failed formatted source code")
		return err
	}

	err = ioutil.WriteFile(localpath, res, 0644)
	if err != nil {
		err = errors.WithMessagef(err, "Failed write source code to file %s", localpath)
		return err
	}

	return nil
}

// AddImport checks for any duplicate imports by name and adds it if not.
func (doc *GoDocument) AddImport(impt GoImport) error {
	impt.Name = strings.Trim(impt.Name, "\"")

	// Get a list of current imports for the document.
	impts, err := doc.Imports()
	if err != nil {
		return err
	}

	// If the document has as the import, don't add it.
	if impts.Has(impt.Name) {
		return nil
	}

	// Loop through all the document root objects for an object of type import.
	// If one exists, append the import to the existing list.
	for _, obj := range doc.Objects().List() {
		if obj.Type != GoObjectType_Import || len(obj.Lines()) == 1 {
			continue
		}
		obj.subLines = append(obj.subLines, impt.String())
		obj.goObjects.list = append(obj.goObjects.list, impt.Object())

		doc.imports = append(doc.imports, impt)

		return nil
	}

	// Document does not have an existing import object, so need to create one and
	// then append to the document.
	newObj := NewObjectImports(impt)

	// Insert after any package, any existing comments or line breaks should be included.
	var insertPos int
	for idx, obj := range doc.Objects().List() {
		switch obj.Type {
		case GoObjectType_Package, GoObjectType_Comment, GoObjectType_LineBreak:
			insertPos = idx
		default:
			break
		}
	}

	// Insert the new import object.
	err = doc.InsertObject(insertPos, newObj)
	if err != nil {
		return err
	}

	return nil
}

// NewObjectImports returns a new GoObject with a single import definition.
func NewObjectImports(impt GoImport) *GoObject {
	lines := []string{
		"import (",
		impt.String(),
		")",
		"",
	}

	obj, _ := ParseGoObject(lines, 0)
	children, err := ParseLines(obj.subLines, 1)
	if err != nil {
		return nil
	}
	for _, child := range children.List() {
		obj.Objects().Add(child)
	}

	return obj
}

// Has checks to see if an import exists by name or alias.
func (impts GoImports) Has(name string) bool {
	for _, impt := range impts {
		if name == impt.Name || (impt.Alias != "" && name == impt.Alias) {
			return true
		}
	}
	return false
}

// Line formats an import as a string.
func (impt GoImport) String() string {
	var imptLine string
	if impt.Alias != "" {
		imptLine = fmt.Sprintf("\t%s \"%s\"", impt.Alias, impt.Name)
	} else {
		imptLine = fmt.Sprintf("\t\"%s\"", impt.Name)
	}
	return imptLine
}

// Object returns the first GoObject for an import.
func (impt GoImport) Object() *GoObject {
	imptObj := NewObjectImports(impt)

	return imptObj.Objects().List()[0]
}

// ParseImportObject extracts all the import definitions.
func ParseImportObject(obj *GoObject) (resp GoImports, err error) {
	if obj.Type != GoObjectType_Import {
		return resp, errors.Errorf("Invalid type %s", string(obj.Type))
	}

	for _, l := range obj.Lines() {
		if !strings.Contains(l, "\"") {
			continue
		}
		l = strings.TrimSpace(l)

		pts := strings.Split(l, "\"")

		var impt GoImport
		if strings.HasPrefix(l, "\"") {
			impt.Name = pts[1]
		} else {
			impt.Alias = strings.TrimSpace(pts[0])
			impt.Name = pts[1]
		}

		resp = append(resp, impt)
	}

	return resp, nil
}
