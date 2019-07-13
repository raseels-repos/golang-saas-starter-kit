package dbtable2crud

import (
	"log"
	"strings"

	"geeks-accelerator/oss/saas-starter-kit/example-project/tools/truss/internal/goparse"
	"github.com/fatih/structtag"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// modelDef defines info about the struct and associated db table.
type modelDef struct {
	*goparse.GoDocument
	Name          string
	TableName     string
	PrimaryField  string
	PrimaryColumn string
	PrimaryType   string
	Fields        []modelField
	FieldNames    []string
	ColumnNames   []string
}

// modelField defines a struct field and associated db column.
type modelField struct {
	ColumnName   string
	DbColumn     *psqlColumn
	FieldName    string
	FieldType    string
	FieldIsPtr   bool
	Tags         *structtag.Tags
	ApiHide      bool
	ApiRead      bool
	ApiCreate    bool
	ApiUpdate    bool
	DefaultValue string
}

// parseModelFile parses the entire model file and then loads the specified model struct.
func parseModelFile(db *sqlx.DB, log *log.Logger, dbName, dbTable, modelFile, modelName string) (*modelDef, error) {

	// Parse the supplied model file.
	doc, err := goparse.ParseFile(log, modelFile)
	if err != nil {
		return nil, err
	}

	// Init new modelDef.
	model := &modelDef{
		GoDocument: doc,
		Name:       modelName,
		TableName:  dbTable,
	}

	// Append the field the the model def.
	model.Fields, err = parseModelFields(doc, modelName, nil)
	if err != nil {
		return nil, err
	}

	for _, sf := range model.Fields {
		model.FieldNames = append(model.FieldNames, sf.FieldName)
		model.ColumnNames = append(model.ColumnNames, sf.ColumnName)
	}

	// Query the database for a table definition.
	dbCols, err := descTable(db, dbName, dbTable)
	if err != nil {
		return model, err
	}

	// Loop over all the database table columns and append to the associated
	// struct field. Don't force all database table columns to be defined in the
	// in the struct.
	for _, dbCol := range dbCols {
		for idx, sf := range model.Fields {
			if sf.ColumnName != dbCol.Column {
				continue
			}

			if dbCol.IsPrimaryKey {
				model.PrimaryColumn = sf.ColumnName
				model.PrimaryField = sf.FieldName
				model.PrimaryType = sf.FieldType
			}

			if dbCol.DefaultValue != nil {
				sf.DefaultValue = *dbCol.DefaultValue

				if dbCol.IsEnum {
					sf.DefaultValue = strings.Trim(sf.DefaultValue, "'")
					sf.DefaultValue = sf.FieldType + "_" + FormatCamel(sf.DefaultValue)
				} else if strings.HasPrefix(sf.DefaultValue, "'") {
					sf.DefaultValue = strings.Trim(sf.DefaultValue, "'")
					sf.DefaultValue = "\"" + sf.DefaultValue + "\""
				}
			}

			c := dbCol
			sf.DbColumn = &c
			model.Fields[idx] = sf
		}
	}

	// Print out the model for debugging.
	//modelJSON, err := json.MarshalIndent(model, "", "    ")
	//if err != nil {
	//	return model, errors.WithStack(err )
	//}
	//log.Printf(string(modelJSON))

	return model, nil
}

// parseModelFields parses the fields from a struct.
func parseModelFields(doc *goparse.GoDocument, modelName string, baseModel *modelDef) ([]modelField, error) {

	// Ensure the model file has a struct with the model name supplied.
	if !doc.HasType(modelName, goparse.GoObjectType_Struct) {
		err := errors.Errorf("Struct with the name %s does not exist", modelName)
		return nil, err
	}

	// Load the struct from parsed go file.
	docModel := doc.Get(modelName, goparse.GoObjectType_Struct)

	// Loop over all the objects contained between the struct definition start and end.
	// This should be a list of variables defined for model.
	resp := []modelField{}
	for _, l := range docModel.Objects().List() {

		// Skip all lines that are not a var.
		if l.Type != goparse.GoObjectType_Line {
			log.Printf("parseModelFile : Model %s has line that is %s, not type line, skipping - %s\n", modelName, l.Type, l.String())
			continue
		}

		// Extract the var name, type and defined tags from the line.
		sv, err := goparse.ParseStructProp(l)
		if err != nil {
			return nil, err
		}

		// Init new modelField for the struct var.
		sf := modelField{
			FieldName:  sv.Name,
			FieldType:  sv.Type,
			FieldIsPtr: strings.HasPrefix(sv.Type, "*"),
			Tags:       sv.Tags,
		}

		// Extract the column name from the var tags.
		if sf.Tags != nil {
			// First try to get the column name from the db tag.
			dbt, err := sf.Tags.Get("db")
			if err != nil && !strings.Contains(err.Error(), "not exist") {
				err = errors.WithStack(err)
				return nil, err
			} else if dbt != nil {
				sf.ColumnName = dbt.Name
			}

			// Second try to get the column name from the json tag.
			if sf.ColumnName == "" {
				jt, err := sf.Tags.Get("json")
				if err != nil && !strings.Contains(err.Error(), "not exist") {
					err = errors.WithStack(err)
					return nil, err
				} else if jt != nil && jt.Name != "-" {
					sf.ColumnName = jt.Name
				}
			}

			var apiActionsSet bool
			tt, err := sf.Tags.Get("truss")
			if err != nil && !strings.Contains(err.Error(), "not exist") {
				err = errors.WithStack(err)
				return nil, err
			} else if tt != nil {
				if tt.Name == "api-create" || tt.HasOption("api-create") {
					sf.ApiCreate = true
					apiActionsSet = true
				}
				if tt.Name == "api-read" || tt.HasOption("api-read") {
					sf.ApiRead = true
					apiActionsSet = true
				}
				if tt.Name == "api-update" || tt.HasOption("api-update") {
					sf.ApiUpdate = true
					apiActionsSet = true
				}
				if tt.Name == "api-hide" || tt.HasOption("api-hide") {
					sf.ApiHide = true
					apiActionsSet = true
				}
			}

			if !apiActionsSet {
				sf.ApiCreate = true
				sf.ApiRead = true
				sf.ApiUpdate = true
			}
		}

		// Set the column name to the field name if empty and does not equal '-'.
		if sf.ColumnName == "" {
			sf.ColumnName = sf.FieldName
		}

		// If a base model as already been parsed with the db columns,
		// append to the current field.
		if baseModel != nil {
			for _, baseSf := range baseModel.Fields {
				if baseSf.ColumnName == sf.ColumnName {
					sf.DefaultValue = baseSf.DefaultValue
					sf.DbColumn = baseSf.DbColumn
					break
				}
			}
		}

		// Append the field the the model def.
		resp = append(resp, sf)
	}

	return resp, nil
}
