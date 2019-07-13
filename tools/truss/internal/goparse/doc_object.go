package goparse

import (
	"log"
	"strings"

	"github.com/fatih/structtag"
	"github.com/pkg/errors"
)

// GoEmptyLine defined a GoObject for a code line break.
var GoEmptyLine = GoObject{
	Type: GoObjectType_LineBreak,
	goObjects: &GoObjects{
		list: []*GoObject{},
	},
}

// GoObjectType defines a set of possible types to group
// parsed code by.
type GoObjectType = string

var (
	GoObjectType_Package   = "package"
	GoObjectType_Import    = "import"
	GoObjectType_Var       = "var"
	GoObjectType_Const     = "const"
	GoObjectType_Func      = "func"
	GoObjectType_Struct    = "struct"
	GoObjectType_Comment   = "comment"
	GoObjectType_LineBreak = "linebreak"
	GoObjectType_Line      = "line"
	GoObjectType_Type      = "type"
)

// GoObject defines a section of code with a nested set of children.
type GoObject struct {
	Type       GoObjectType
	Name       string
	startLines []string
	endLines   []string
	subLines   []string
	goObjects  *GoObjects
	Index      int
}

// GoObjects stores a list of GoObject.
type GoObjects struct {
	list []*GoObject
}

// Objects returns the list of *GoObject.
func (obj *GoObject) Objects() *GoObjects {
	if obj.goObjects == nil {
		obj.goObjects = &GoObjects{
			list: []*GoObject{},
		}
	}
	return obj.goObjects
}

// Clone performs a deep copy of the struct.
func (obj *GoObject) Clone() *GoObject {
	n := &GoObject{
		Type:       obj.Type,
		Name:       obj.Name,
		startLines: obj.startLines,
		endLines:   obj.endLines,
		subLines:   obj.subLines,
		goObjects: &GoObjects{
			list: []*GoObject{},
		},
		Index: obj.Index,
	}
	for _, sub := range obj.Objects().List() {
		n.Objects().Add(sub.Clone())
	}
	return n
}

// IsComment returns whether an object is of type GoObjectType_Comment.
func (obj *GoObject) IsComment() bool {
	if obj.Type != GoObjectType_Comment {
		return false
	}
	return true
}

// Contains searches all the lines for the object for a matching string.
func (obj *GoObject) Contains(match string) bool {
	for _, l := range obj.Lines() {
		if strings.Contains(l, match) {
			return true
		}
	}
	return false
}

// UpdateLines parses the new code and replaces the current GoObject.
func (obj *GoObject) UpdateLines(newLines []string) error {

	// Parse the new lines.
	objs, err := ParseLines(newLines, 0)
	if err != nil {
		return err
	}

	var newObj *GoObject
	for _, obj := range objs.List() {
		if obj.Type == GoObjectType_LineBreak {
			continue
		}

		if newObj == nil {
			newObj = obj
		}

		// There should only be one resulting parsed object that is
		// not of type GoObjectType_LineBreak.
		return errors.New("Can only update single blocks of code")
	}

	// No new code was parsed, return error.
	if newObj == nil {
		return errors.New("Failed to render replacement code")
	}

	return obj.Update(newObj)
}

// Update performs a deep copy that overwrites the existing values.
func (obj *GoObject) Update(newObj *GoObject) error {
	obj.Type = newObj.Type
	obj.Name = newObj.Name
	obj.startLines = newObj.startLines
	obj.endLines = newObj.endLines
	obj.subLines = newObj.subLines
	obj.goObjects = newObj.goObjects
	return nil
}

// Lines returns a list of strings for current object and all children.
func (obj *GoObject) Lines() []string {
	l := []string{}

	// First include any lines before the sub objects.
	for _, sl := range obj.startLines {
		l = append(l, sl)
	}

	// If there are parsed sub objects include those lines else when
	// no sub objects, just use the sub lines.
	if len(obj.Objects().List()) > 0 {
		for _, sl := range obj.Objects().Lines() {
			l = append(l, sl)
		}
	} else {
		for _, sl := range obj.subLines {
			l = append(l, sl)
		}
	}

	// Lastly include any other lines that are after all parsed sub objects.
	for _, sl := range obj.endLines {
		l = append(l, sl)
	}

	return l
}

// String returns the lines separated by line break.
func (obj *GoObject) String() string {
	return strings.Join(obj.Lines(), "\n")
}

// Lines returns a list of strings for all the list objects.
func (objs *GoObjects) Lines() []string {
	l := []string{}
	for _, obj := range objs.List() {
		for _, oj := range obj.Lines() {
			l = append(l, oj)
		}
	}
	return l
}

// String returns all the lines for the list objects.
func (objs *GoObjects) String() string {
	lines := []string{}
	for _, obj := range objs.List() {
		lines = append(lines, obj.String())
	}
	return strings.Join(lines, "\n")
}

// List returns the list of GoObjects.
func (objs *GoObjects) List() []*GoObject {
	return objs.list
}

// HasFunc searches the current list of objects for a function object by name.
func (objs *GoObjects) HasFunc(name string) bool {
	return objs.HasType(name, GoObjectType_Func)
}

// Get returns the GoObject for the matching name and type.
func (objs *GoObjects) Get(name string, objType GoObjectType) *GoObject {
	for _, obj := range objs.list {
		if obj.Name == name && (objType == "" || obj.Type == objType) {
			return obj
		}
	}
	return nil
}

// HasType checks is a GoObject exists for the matching name and type.
func (objs *GoObjects) HasType(name string, objType GoObjectType) bool {
	for _, obj := range objs.list {
		if obj.Name == name && (objType == "" || obj.Type == objType) {
			return true
		}
	}
	return false
}

// HasObject checks to see if the exact code block exists.
func (objs *GoObjects) HasObject(src *GoObject) bool {
	if src == nil {
		return false
	}

	// Generate the code for the supplied object.
	srcLines := []string{}
	for _, l := range src.Lines() {
		// Exclude empty lines.
		l = strings.TrimSpace(l)
		if l != "" {
			srcLines = append(srcLines, l)
		}
	}
	srcStr := strings.Join(srcLines, "\n")

	// Loop over all the objects and match with src code.
	for _, obj := range objs.list {
		objLines := []string{}
		for _, l := range obj.Lines() {
			// Exclude empty lines.
			l = strings.TrimSpace(l)
			if l != "" {
				objLines = append(objLines, l)
			}
		}
		objStr := strings.Join(objLines, "\n")

		// Return true if the current object code matches src code.
		if srcStr == objStr {
			return true
		}
	}

	return false
}

// Add appends a new GoObject to the list.
func (objs *GoObjects) Add(newObj *GoObject) error {
	newObj.Index = len(objs.list)
	objs.list = append(objs.list, newObj)
	return nil
}

// Insert appends a new GoObject at the desired position to the list.
func (objs *GoObjects) Insert(pos int, newObj *GoObject) error {
	newList := []*GoObject{}

	var newIdx int
	for _, obj := range objs.list {
		if obj.Index < pos {
			obj.Index = newIdx
			newList = append(newList, obj)
		} else {
			if obj.Index == pos {
				newObj.Index = newIdx
				newList = append(newList, newObj)
				newIdx++
			}
			obj.Index = newIdx
			newList = append(newList, obj)
		}

		newIdx++
	}

	objs.list = newList

	return nil
}

// Remove deletes a GoObject from the list.
func (objs *GoObjects) Remove(delObjs ...*GoObject) error {
	for _, delObj := range delObjs {
		oldList := objs.List()
		objs.list = []*GoObject{}

		var newIdx int
		for _, obj := range oldList {
			if obj.Index == delObj.Index {
				continue
			}
			obj.Index = newIdx
			objs.list = append(objs.list, obj)
			newIdx++
		}
	}

	return nil
}

// Replace updates an existing GoObject while maintaining is same position.
func (objs *GoObjects) Replace(oldObj *GoObject, newObjs ...*GoObject) error {
	if oldObj.Index >= len(objs.list) {
		return errors.WithStack(errGoObjectNotExist)
	} else if len(newObjs) == 0 {
		return nil
	}

	oldList := objs.List()
	objs.list = []*GoObject{}

	var newIdx int
	for _, obj := range oldList {
		if obj.Index < oldObj.Index {
			obj.Index = newIdx
			objs.list = append(objs.list, obj)
			newIdx++
		} else if obj.Index == oldObj.Index {
			for _, newObj := range newObjs {
				newObj.Index = newIdx
				objs.list = append(objs.list, newObj)
				newIdx++
			}
		} else {
			obj.Index = newIdx
			objs.list = append(objs.list, obj)
			newIdx++
		}
	}

	return nil
}

// ReplaceFuncByName finds an existing GoObject with type GoObjectType_Func by name
// and then performs a replace with the supplied new GoObject.
func (objs *GoObjects) ReplaceFuncByName(name string, fn *GoObject) error {
	return objs.ReplaceTypeByName(name, fn, GoObjectType_Func)
}

// ReplaceTypeByName finds an existing GoObject with type by name
// and then performs a replace with the supplied new GoObject.
func (objs *GoObjects) ReplaceTypeByName(name string, newObj *GoObject, objType GoObjectType) error {
	if newObj.Name == "" {
		newObj.Name = name
	}
	if newObj.Type == "" && objType != "" {
		newObj.Type = objType
	}

	for _, obj := range objs.list {
		if obj.Name == name && (objType == "" || objType == obj.Type) {
			return objs.Replace(obj, newObj)
		}
	}
	return errors.WithStack(errGoObjectNotExist)
}

// Empty determines if all the GoObject in the list are line breaks.
func (objs *GoObjects) Empty() bool {
	var hasStuff bool
	for _, obj := range objs.List() {
		switch obj.Type {
		case GoObjectType_LineBreak:
		//case GoObjectType_Comment:
		//case GoObjectType_Import:
		// do nothing
		default:
			hasStuff = true
		}
	}
	return hasStuff
}

// Debug prints out the GoObject to logger.
func (obj *GoObject) Debug(log *log.Logger) {
	log.Println(obj.Name)
	log.Println(" > type:", obj.Type)
	log.Println(" > start lines:")
	for _, l := range obj.startLines {
		log.Println("   ", l)
	}

	log.Println(" > sub lines:")
	for _, l := range obj.subLines {
		log.Println("   ", l)
	}

	log.Println(" > end lines:")
	for _, l := range obj.endLines {
		log.Println("   ", l)
	}
}

// Defines a property of a struct.
type structProp struct {
	Name string
	Type string
	Tags *structtag.Tags
}

// ParseStructProp extracts the details for a struct property.
func ParseStructProp(obj *GoObject) (structProp, error) {

	if obj.Type != GoObjectType_Line {
		return structProp{}, errors.Errorf("Unable to parse object of type %s", obj.Type)
	}

	// Remove any white space from the code line.
	ls := strings.TrimSpace(strings.Join(obj.Lines(), " "))

	// Extract the property name and type for the line.
	// ie: ID            string         `json:"id"`
	var resp structProp
	for _, p := range strings.Split(ls, " ") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if resp.Name == "" {
			resp.Name = p
		} else if resp.Type == "" {
			resp.Type = p
		} else {
			break
		}
	}

	// If the line contains tags, extract and parse them.
	if strings.Contains(ls, "`") {
		tagStr := strings.Split(ls, "`")[1]

		var err error
		resp.Tags, err = structtag.Parse(tagStr)
		if err != nil {
			err = errors.WithMessagef(err, "Failed to parse struct tag for field %s: %s", resp.Name, tagStr)
			return structProp{}, err
		}
	}

	return resp, nil
}
