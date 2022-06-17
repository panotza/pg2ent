package scepter

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"go/format"
	"io"
	"log"
	"reflect"
	"strings"
	"text/template"

	"github.com/panotza/pg2ent/postgres"
	"github.com/samber/lo"
)

var (
	ErrSkipField = errors.New("scepter: skip field")
)

//go:embed schema.tmpl
var schemaTmpl string

type Config struct {
	SQLFile string `yaml:"sql-file"`
	OutDir  string `yaml:"out-dir"`
	Format  bool   `yaml:"format"`
	Rule    struct {
		NoDefault        bool     `yaml:"no-default"`
		ImmutableColumns []string `yaml:"immutable-columns"`
	} `yaml:"rule"`
	Type      map[string]string     `yaml:"type"`
	Define    map[string]DefineType `yaml:"define"`
	Overrides []OverrideBehavior    `yaml:"overrides"`
}

type DefineType struct {
	Field      string   `yaml:"field"`
	Imports    []string `yaml:"imports"`
	GoType     string   `yaml:"go-type"`
	SchemaType string   `yaml:"schema-type"`
	Default    string   `yaml:"default"`
}

type OverrideBehavior struct {
	Name     string    `yaml:"name"`
	Matchers []Matcher `yaml:"matchers"`
	With     string    `yaml:"with"`
	Imports  []string  `yaml:"imports"`
}

type Matcher struct {
	Prop  string `yaml:"prop"`
	Value any    `yaml:"value"`
}

type Scepter struct {
	c    Config
	tmpl *template.Template
}

func NewScepter(c Config) *Scepter {
	t := template.Must(template.New("").Parse(schemaTmpl))

	return &Scepter{
		c:    c,
		tmpl: t,
	}
}

type generateContext struct {
	importMemo map[string]struct{}
}

func (s *Scepter) generateAnnotation(ctx *generateContext, table postgres.Table) []string {
	var ss []string
	// detect composite primary key
	compositeColumns := lo.Filter(table.Columns, func(x postgres.Column, _ int) bool {
		return x.IsPrimary && x.ForeignKey != nil
	})
	if len(compositeColumns) > 1 {
		keys := lo.Map(compositeColumns, func(x postgres.Column, _ int) string {
			return `"` + x.Name + `"`
		})
		ss = append(ss, fmt.Sprintf(`field.ID(%s)`, strings.Join(keys, ", ")))
	}
	return ss
}

func (s *Scepter) generateField(ctx *generateContext, table postgres.Table, column postgres.Column) (string, error) {
	if column.Name == "id" && column.IsPrimary && column.Type != "uuid" {
		return "", ErrSkipField
	}

	var ss []string
	v := reflect.ValueOf(column)
	for _, override := range s.c.Overrides {
		matched := true
		for _, matcher := range override.Matchers {
			fieldValue := v.FieldByName(matcher.Prop)
			if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() && matcher.Value == nil {
				continue
			}
			if !reflect.DeepEqual(fieldValue.Interface(), matcher.Value) {
				matched = false
				break
			}
		}

		if matched {
			log.Printf(`override "%s"."%s" with %s`, table.Name, column.Name, override.Name)
			if len(override.Imports) > 0 {
				for _, x := range override.Imports {
					ctx.importMemo[x] = struct{}{}
				}
			}
			return strings.Replace(override.With, "%s", column.Name, 1), nil
		}
	}

	fieldStr, ok := s.c.Type[column.Type]
	if !ok {
		return "", fmt.Errorf(`mapping for "%s"."%s" type "%s" not found in the configuration`, table.Name, column.Name, column.Type)
	}

	defaultStr := "Default(%s)"

	defType, ok := s.c.Define[fieldStr]
	if ok {
		ss = append(ss, strings.Replace(defType.Field, "%s", column.Name, 1))
		if defType.GoType != "" {
			ss = append(ss, fmt.Sprintf("GoType(%s)", defType.GoType))
		}
		if defType.SchemaType != "" {
			ss = append(ss, fmt.Sprintf("SchemaType(%s)", defType.SchemaType))
		}
		if defType.Default != "" {
			defaultStr = defType.Default
		}
		if len(defType.Imports) > 0 {
			for _, x := range defType.Imports {
				ctx.importMemo[x] = struct{}{}
			}
		}
	} else {
		ss = append(ss, strings.Replace(fieldStr, "%s", column.Name, 1))
	}

	if !column.IsNotNull {
		ss = append(ss, "Nillable()")
		ss = append(ss, "Optional()")
	}

	if column.DefaultType != nil {
		if column.IsUnique {
			return "", fmt.Errorf(`"%s"."%s" is unique it cannot have default values`, table.Name, column.Name)
		}
		if !column.IsPrimary {
			if !lo.Contains(ss, "Optional()") {
				ss = append(ss, "Optional()")
			}
			if !s.c.Rule.NoDefault {
				ss = append(ss, strings.Replace(defaultStr, "%s", column.DefaultType.Value, 1))
			}
			if strings.ToLower(column.DefaultType.Type) == "func" {
				ctx.importMemo["entgo.io/ent/dialect/entsql"] = struct{}{}
				ss = append(ss, fmt.Sprintf("Annotations(&entsql.Annotation{\n                Default: \"%s\",\n            })", column.DefaultType.Value+"()"))
			} else if strings.ToLower(column.DefaultType.Type) == "sqlvaluefunction" {
				ctx.importMemo["entgo.io/ent/dialect/entsql"] = struct{}{}
				ss = append(ss, fmt.Sprintf("Annotations(&entsql.Annotation{\n                Default: \"%s\",\n            })", column.DefaultType.Value))
			}
		}
	}

	if column.IsUnique {
		ss = append(ss, "Unique()")
	}
	if lo.Contains(s.c.Rule.ImmutableColumns, column.Name) {
		ss = append(ss, "Immutable()")
	}
	return strings.Join(ss, ".\n"), nil
}

func (s *Scepter) generateIndex(ctx *generateContext, table postgres.Table, index postgres.Index) (string, error) {
	var ss []string

	columns := lo.Map(index.Columns, func(t string, _ int) string {
		return `"` + t + `"`
	})
	ss = append(ss, fmt.Sprintf(`index.Fields(%s)`, strings.Join(columns, ", ")))
	if index.IsUnique {
		ss = append(ss, "Unique()")
	}
	return strings.Join(ss, ".\n"), nil
}

type TemplateData struct {
	Import      []string
	SchemaName  string
	Annotations []string
	Fields      []string
	Indexes     []string
}

func (s *Scepter) Generate(w io.Writer, table postgres.Table) error {
	var data TemplateData
	data.SchemaName = pascal(Singularize(table.Name))

	ctx := &generateContext{
		importMemo: make(map[string]struct{}),
	}

	data.Annotations = s.generateAnnotation(ctx, table)

	for _, col := range table.Columns {
		x, err := s.generateField(ctx, table, col)
		if err != nil {
			if errors.Is(err, ErrSkipField) {
				continue
			}
			return err
		}
		data.Fields = append(data.Fields, x)
	}

	for _, index := range table.Indexes {
		x, err := s.generateIndex(ctx, table, index)
		if err != nil {
			return err
		}
		data.Indexes = append(data.Indexes, x)
	}

	data.Import = lo.Keys(ctx.importMemo)

	var buf bytes.Buffer
	err := s.tmpl.Execute(&buf, data)
	if err != nil {
		return fmt.Errorf("execute tmpl file error: %w", err)
	}
	if s.c.Format {
		b, err := format.Source(buf.Bytes())
		if err != nil {
			return fmt.Errorf("format file %s error: %w", table.Name, err)
		}
		buf.Reset()
		buf.Write(b)
	}
	_, err = io.Copy(w, &buf)
	return err
}
