package dbtable2crud

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"geeks-accelerator/oss/saas-starter-kit/tools/truss/internal/goparse"
	"github.com/dustin/go-humanize/english"
	"github.com/fatih/camelcase"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// Run in the main entry point for the dbtable2crud cmd.
func Run(db *sqlx.DB, log *log.Logger, dbName, dbTable, modelFile, modelName, templateDir, goSrcPath string, saveChanges bool) error {
	log.SetPrefix(log.Prefix() + " : dbtable2crud")

	// Ensure the schema is up to date
	if err := schema.Migrate(db, log); err != nil {
		return err
	}

	// When dbTable is empty, lower case the model name
	if dbTable == "" {
		dbTable = strings.Join(camelcase.Split(modelName), " ")
		dbTable = english.PluralWord(2, dbTable, "")
		dbTable = strings.Replace(dbTable, " ", "_", -1)
		dbTable = strings.ToLower(dbTable)
	}

	// Parse the model file and load the specified model struct.
	model, err := parseModelFile(db, log, dbName, dbTable, modelFile, modelName)
	if err != nil {
		return err
	}

	// Basic lint of the model struct.
	err = validateModel(log, model)
	if err != nil {
		return err
	}

	tmplData := map[string]interface{}{
		"GoSrcPath": goSrcPath,
	}

	// Update the model file with new or updated code.
	err = updateModel(log, model, modelFile, templateDir, tmplData, saveChanges)
	if err != nil {
		return err
	}

	// Update the model crud file with new or updated code.
	err = updateModelCrud(db, log, dbName, dbTable, modelFile, modelName, templateDir, model, tmplData, saveChanges)
	if err != nil {
		return err
	}

	return nil
}

// validateModel performs a basic lint of the model struct to ensure
// code gen output is correct.
func validateModel(log *log.Logger, model *modelDef) error {
	for _, sf := range model.Fields {
		if sf.DbColumn == nil && sf.ColumnName != "-" {
			log.Printf("validateStruct : Unable to find struct field for db column %s\n", sf.ColumnName)
		}

		var expectedType string
		switch sf.FieldName {
		case "ID":
			expectedType = "string"
		case "CreatedAt":
			expectedType = "time.Time"
		case "UpdatedAt":
			expectedType = "time.Time"
		case "ArchivedAt":
			expectedType = "pq.NullTime"
		}

		if expectedType != "" && expectedType != sf.FieldType {
			log.Printf("validateStruct : Struct field %s should be of type %s not %s\n", sf.FieldName, expectedType, sf.FieldType)
		}
	}

	return nil
}

// updateModel updated the parsed code file with the new code.
func updateModel(log *log.Logger, model *modelDef, modelFile, templateDir string, tmplData map[string]interface{}, saveChanges bool) error {

	// Execute template and parse code to be used to compare against modelFile.
	tmplObjs, err := loadTemplateObjects(log, model, templateDir, "models.tmpl", tmplData)
	if err != nil {
		return err
	}

	// Store the current code as a string to produce a diff.
	curCode := model.String()

	objHeaders := []*goparse.GoObject{}

	for _, obj := range tmplObjs {
		if obj.Type == goparse.GoObjectType_Comment || obj.Type == goparse.GoObjectType_LineBreak {
			objHeaders = append(objHeaders, obj)
			continue
		}

		if model.HasType(obj.Name, obj.Type) {
			cur := model.Objects().Get(obj.Name, obj.Type)

			newObjs := []*goparse.GoObject{}
			if len(objHeaders) > 0 {
				// Remove any comments and linebreaks before the existing object so updates can be added.
				removeObjs := []*goparse.GoObject{}
				for idx := cur.Index - 1; idx > 0; idx-- {
					prevObj := model.Objects().List()[idx]
					if prevObj.Type == goparse.GoObjectType_Comment || prevObj.Type == goparse.GoObjectType_LineBreak {
						removeObjs = append(removeObjs, prevObj)
					} else {
						break
					}
				}

				if len(removeObjs) > 0 {
					err := model.Objects().Remove(removeObjs...)
					if err != nil {
						err = errors.WithMessagef(err, "Failed to update object %s %s for %s", obj.Type, obj.Name, model.Name)
						return err
					}

					// Make sure the current index is correct.
					cur = model.Objects().Get(obj.Name, obj.Type)
				}

				// Append comments and line breaks before adding the object
				for _, c := range objHeaders {
					newObjs = append(newObjs, c)
				}
			}

			newObjs = append(newObjs, obj)

			// Do the object replacement.
			err := model.Objects().Replace(cur, newObjs...)
			if err != nil {
				err = errors.WithMessagef(err, "Failed to update object %s %s for %s", obj.Type, obj.Name, model.Name)
				return err
			}
		} else {
			// Append comments and line breaks before adding the object
			for _, c := range objHeaders {
				err := model.Objects().Add(c)
				if err != nil {
					err = errors.WithMessagef(err, "Failed to add object %s %s for %s", c.Type, c.Name, model.Name)
					return err
				}
			}

			err := model.Objects().Add(obj)
			if err != nil {
				err = errors.WithMessagef(err, "Failed to add object %s %s for %s", obj.Type, obj.Name, model.Name)
				return err
			}
		}

		objHeaders = []*goparse.GoObject{}
	}

	// Set some flags to determine additional imports and need to be added.
	var hasEnum bool
	var hasPq bool
	for _, f := range model.Fields {
		if f.DbColumn != nil && f.DbColumn.IsEnum {
			hasEnum = true
		}
		if strings.HasPrefix(strings.Trim(f.FieldType, "*"), "pq.") {
			hasPq = true
		}
	}

	reqImports := []string{}
	if hasEnum {
		reqImports = append(reqImports, "database/sql/driver")
		reqImports = append(reqImports, "gopkg.in/go-playground/validator.v9")
		reqImports = append(reqImports, "github.com/pkg/errors")
	}

	if hasPq {
		reqImports = append(reqImports, "github.com/lib/pq")
	}

	for _, in := range reqImports {
		err := model.AddImport(goparse.GoImport{Name: in})
		if err != nil {
			err = errors.WithMessagef(err, "Failed to add import %s for %s", in, model.Name)
			return err
		}
	}

	if saveChanges {
		err = model.Save(modelFile)
		if err != nil {
			err = errors.WithMessagef(err, "Failed to save changes for %s to %s", model.Name, modelFile)
			return err
		}
	} else {
		// Produce a diff after the updates have been applied.
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(curCode, model.String(), true)
		fmt.Println(dmp.DiffPrettyText(diffs))
	}

	return nil
}

// updateModelCrud updated the parsed code file with the new code.
func updateModelCrud(db *sqlx.DB, log *log.Logger, dbName, dbTable, modelFile, modelName, templateDir string, baseModel *modelDef, tmplData map[string]interface{}, saveChanges bool) error {

	// Load all the updated struct fields from the base model file.
	structFields := make(map[string]map[string]modelField)
	for _, obj := range baseModel.GoDocument.Objects().List() {
		if obj.Type != goparse.GoObjectType_Struct || obj.Name == baseModel.Name {
			continue
		}

		objFields, err := parseModelFields(baseModel.GoDocument, obj.Name, baseModel)
		if err != nil {
			return err
		}

		structFields[obj.Name] = make(map[string]modelField)
		for _, f := range objFields {
			structFields[obj.Name][f.FieldName] = f
		}
	}

	// Append the struct fields to be used for template execution.
	if tmplData == nil {
		tmplData = make(map[string]interface{})
	}
	tmplData["StructFields"] = structFields

	// Get the dir to store crud methods and test files.
	modelDir := filepath.Dir(modelFile)

	// Process the CRUD hanlders template and write to file.
	crudFilePath := filepath.Join(modelDir, FormatCamelLowerUnderscore(baseModel.Name)+".go")
	crudTmplFile := "model_crud.tmpl"
	err := updateModelCrudFile(db, log, dbName, dbTable, templateDir, crudFilePath, crudTmplFile, baseModel, tmplData, saveChanges)
	if err != nil {
		return err
	}

	// Process the CRUD test template and write to file.
	testFilePath := filepath.Join(modelDir, FormatCamelLowerUnderscore(baseModel.Name)+"_test.go")
	testTmplFile := "model_crud_test.tmpl"
	err = updateModelCrudFile(db, log, dbName, dbTable, templateDir, testFilePath, testTmplFile, baseModel, tmplData, saveChanges)
	if err != nil {
		return err
	}

	return nil
}

// updateModelCrudFile processes the input file.
func updateModelCrudFile(db *sqlx.DB, log *log.Logger, dbName, dbTable, templateDir, crudFilePath, tmplFile string, baseModel *modelDef, tmplData map[string]interface{}, saveChanges bool) error {

	// Execute template and parse code to be used to compare against modelFile.
	tmplObjs, err := loadTemplateObjects(log, baseModel, templateDir, tmplFile, tmplData)
	if err != nil {
		return err
	}

	var crudDoc *goparse.GoDocument
	if _, err := os.Stat(crudFilePath); os.IsNotExist(err) {
		crudDoc, err = goparse.NewGoDocument(baseModel.Package)
		if err != nil {
			return err
		}
	} else {
		// Parse the supplied model file.
		crudDoc, err = goparse.ParseFile(log, crudFilePath)
		if err != nil {
			return err
		}
	}

	// Store the current code as a string to produce a diff.
	curCode := crudDoc.String()

	objHeaders := []*goparse.GoObject{}

	for _, obj := range tmplObjs {
		if obj.Type == goparse.GoObjectType_Comment || obj.Type == goparse.GoObjectType_LineBreak {
			objHeaders = append(objHeaders, obj)
			continue
		}

		if obj.Name == "" && (obj.Type == goparse.GoObjectType_Var || obj.Type == goparse.GoObjectType_Const) {
			var curDocObj *goparse.GoObject
			for _, subObj := range obj.Objects().List() {
				for _, do := range crudDoc.Objects().List() {
					if do.Name == "" && (do.Type == goparse.GoObjectType_Var || do.Type == goparse.GoObjectType_Const) {
						for _, subDocObj := range do.Objects().List() {
							if subDocObj.String() == subObj.String() && subObj.Type != goparse.GoObjectType_LineBreak {
								curDocObj = do
								break
							}

						}
					}
				}
			}

			if curDocObj != nil {
				for _, subObj := range obj.Objects().List() {
					var hasSubObj bool
					for _, subDocObj := range curDocObj.Objects().List() {
						if subDocObj.String() == subObj.String() {
							hasSubObj = true
							break
						}
					}

					if !hasSubObj {
						curDocObj.Objects().Add(subObj)
						if err != nil {
							err = errors.WithMessagef(err, "Failed to add object %s %s for %s", obj.Type, obj.Name, baseModel.Name)
							return err
						}
					}
				}
			} else {
				// Append comments and line breaks before adding the object
				for _, c := range objHeaders {
					err := crudDoc.Objects().Add(c)
					if err != nil {
						err = errors.WithMessagef(err, "Failed to add object %s %s for %s", c.Type, c.Name, baseModel.Name)
						return err
					}
				}

				err := crudDoc.Objects().Add(obj)
				if err != nil {
					err = errors.WithMessagef(err, "Failed to add object %s %s for %s", obj.Type, obj.Name, baseModel.Name)
					return err
				}
			}
		} else if crudDoc.HasType(obj.Name, obj.Type) {
			cur := crudDoc.Objects().Get(obj.Name, obj.Type)

			newObjs := []*goparse.GoObject{}
			if len(objHeaders) > 0 {
				// Remove any comments and linebreaks before the existing object so updates can be added.
				removeObjs := []*goparse.GoObject{}
				for idx := cur.Index - 1; idx > 0; idx-- {
					prevObj := crudDoc.Objects().List()[idx]
					if prevObj.Type == goparse.GoObjectType_Comment || prevObj.Type == goparse.GoObjectType_LineBreak {
						removeObjs = append(removeObjs, prevObj)
					} else {
						break
					}
				}

				if len(removeObjs) > 0 {
					err := crudDoc.Objects().Remove(removeObjs...)
					if err != nil {
						err = errors.WithMessagef(err, "Failed to update object %s %s for %s", obj.Type, obj.Name, baseModel.Name)
						return err
					}

					// Make sure the current index is correct.
					cur = crudDoc.Objects().Get(obj.Name, obj.Type)
				}

				// Append comments and line breaks before adding the object
				for _, c := range objHeaders {
					newObjs = append(newObjs, c)
				}
			}

			newObjs = append(newObjs, obj)

			// Do the object replacement.
			err := crudDoc.Objects().Replace(cur, newObjs...)
			if err != nil {
				err = errors.WithMessagef(err, "Failed to update object %s %s for %s", obj.Type, obj.Name, baseModel.Name)
				return err
			}
		} else {
			// Append comments and line breaks before adding the object
			for _, c := range objHeaders {
				err := crudDoc.Objects().Add(c)
				if err != nil {
					err = errors.WithMessagef(err, "Failed to add object %s %s for %s", c.Type, c.Name, baseModel.Name)
					return err
				}
			}

			err := crudDoc.Objects().Add(obj)
			if err != nil {
				err = errors.WithMessagef(err, "Failed to add object %s %s for %s", obj.Type, obj.Name, baseModel.Name)
				return err
			}
		}

		objHeaders = []*goparse.GoObject{}
	}

	if saveChanges {
		err = crudDoc.Save(crudFilePath)
		if err != nil {
			err = errors.WithMessagef(err, "Failed to save changes for %s to %s", baseModel.Name, crudFilePath)
			return err
		}
	} else {
		// Produce a diff after the updates have been applied.
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(curCode, crudDoc.String(), true)
		fmt.Println(dmp.DiffPrettyText(diffs))
	}

	return nil
}
