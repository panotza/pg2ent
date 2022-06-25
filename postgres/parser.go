package postgres

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	pgQuery "github.com/pganalyze/pg_query_go"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
)

func Tree(sql string) (string, error) {
	return pgQuery.ParseToJSON(sql)
}

func ParseSQL(sql string) []Table {
	tree, err := Tree(sql)
	if err != nil {
		panic(err)
	}

	var tables []Table
	statements := gjson.Get(tree, "#.RawStmt.stmt").Array()
	for _, stmt := range statements {
		stmt.ForEach(func(key, value gjson.Result) bool {
			switch key.String() {
			case "CreateStmt":
				table := parseCreateStmt(value)
				tables = append(tables, table)
			case "IndexStmt":
				table := value.Get("relation.RangeVar.relname").String()
				if table == "" {
					panic("table name is not found in create index statement")
				}
				_, tableIndex, ok := lo.FindIndexOf(tables, func(x Table) bool {
					return x.Name == table
				})
				if !ok {
					panic(fmt.Sprintf("cannot find %s table to create unique index", table))
				}

				isUnique := value.Get("unique").Bool()
				elems := value.Get("indexParams.#.IndexElem.name").Array()
				isComposite := len(elems) > 1

				var columns []string
				if !isComposite {
					column := elems[0].String()
					_, columnIndex, ok := lo.FindIndexOf(tables[tableIndex].Columns, func(x Column) bool {
						return x.Name == column
					})
					if !ok {
						panic(fmt.Sprintf("cannot find %s column in table %s to create unique index", column, table))
					}

					tables[tableIndex].Columns[columnIndex].IsUnique = isUnique
				} else {
					for _, elem := range elems {
						columns = append(columns, elem.String())
					}

					tables[tableIndex].Indexes = append(tables[tableIndex].Indexes, Index{
						Columns:  columns,
						IsUnique: isUnique,
					})
				}
			}
			return true
		})
	}

	for i := range tables {
		tables[i].Finalize()
	}
	return tables
}

func parseCreateStmt(createStmt gjson.Result) Table {
	tableName := createStmt.Get("relation.RangeVar.relname").String()

	table := Table{Name: tableName}

	elems := createStmt.Get("tableElts").Array()
	for _, elem := range elems {
		defType, value := func(elem gjson.Result) (string, gjson.Result) {
			var (
				retKey   string
				retValue gjson.Result
			)
			elem.ForEach(func(key, value gjson.Result) bool {
				retKey = key.String()
				retValue = value
				return false
			})
			return retKey, retValue
		}(elem)

		switch defType {
		case "ColumnDef":
			column := Column{}
			column.Name = value.Get("colname").String()
			column.Type = parseJsonColumnTypeName(value.Get("typeName.TypeName.names"))

			constraints := value.Get("constraints")
			if constraints.Exists() {
				for _, con := range constraints.Array() {
					conType := con.Get("Constraint.contype").Int()
					switch conType {
					case conTypeIsNotNull:
						column.IsNotNull = true
					case conTypeDefault:
						expr := con.Get("Constraint.raw_expr")
						expr.ForEach(func(valType, value gjson.Result) bool {
							t := parseJsonValueTypeExpr(valType, value.Raw)
							column.DefaultType = &t
							return true
						})
					}
				}
			}

			table.Columns = append(table.Columns, column)
		case "Constraint":
			conType := value.Get("contype").Int()
			switch conType {
			case conTypePrimaryKey:
				pkColumns := value.Get("keys.#.String.str").Array()
				for _, key := range pkColumns {
					_, i, ok := lo.FindIndexOf(table.Columns, func(col Column) bool {
						return col.Name == key.String()
					})
					if !ok {
						panic(fmt.Sprintf("cannot find %s column in table %s to set primary key", key, table.Name))
					}
					table.Columns[i].IsNotNull = true
					table.Columns[i].IsPrimary = true
				}
			case conTypeForeignKey:
				toTable := value.Get("pktable.RangeVar.relname").String()
				if toTable == "" {
					panic(fmt.Sprintf("foreign key to table name not found"))
				}
				toTableColumn := value.Get("pk_attrs.0.String.str").String()
				if toTableColumn == "" {
					panic(fmt.Sprintf("foreign key to table column not found"))
				}

				fkColumn := value.Get("fk_attrs.0.String.str").String()
				if fkColumn == "" {
					panic(fmt.Sprintf("foreign key column not found"))
				}

				_, i, ok := lo.FindIndexOf(table.Columns, func(col Column) bool {
					return col.Name == fkColumn
				})
				if !ok {
					panic(fmt.Sprintf("cannot find %s column in table %s to set foreign key", fkColumn, table.Name))
				}

				table.Columns[i].ForeignKey = &foreignKey{
					Column: toTableColumn,
					Table:  toTable,
				}
			}
		}
	}
	return table
}

func parseJsonValueTypeExpr(result gjson.Result, val string) defaultType {
	switch result.String() {
	case "A_Const":
		val := gjson.Get(val, "val")
		if !val.Exists() {
			log.Panicf("val is not exist in default expr")
		}
		var x defaultType
		val.ForEach(func(t, value gjson.Result) bool {
			x.Type = t.String()
			value.ForEach(func(key, value gjson.Result) bool {
				x.Value = value.Raw
				return true
			})
			return true
		})
		return x
	case "FuncCall":
		// TODO: parse function arguments
		var x defaultType
		funcName := gjson.Get(val, "funcname.0.String.str").String()
		if funcName == "" {
			panic("func name not found")
		}
		x.Type = "Func"
		x.Value = funcName
		return x
	case "TypeCast":
		var x defaultType
		arg := gjson.Get(val, "arg")
		if !arg.Exists() {
			panic("arg not found")
		}
		arg.ForEach(func(valType, value gjson.Result) bool {
			x = parseJsonValueTypeExpr(valType, value.Raw)
			return true
		})
		x.Type = parseJsonColumnTypeName(gjson.Get(val, "typeName.TypeName.names"))
		return x
	case "SQLValueFunction":
		var x defaultType
		x.Type = "SQLValueFunction"
		op := gjson.Get(val, "op").String()
		prec := gjson.Get(val, "typmod").String()
		constant, ok := SQLValueFunctionOpToConstant[op]
		if !ok {
			panic(fmt.Sprintf("SQLValueFunction op %s is not supported", op))
		}
		x.Value = strings.Replace(constant, "%s", prec, 1)
		return x
	}
	panic(fmt.Sprintf("unreachable %s", result))
}

func parseJsonColumnTypeName(typeNames gjson.Result) string {
	typeIndex := len(typeNames.Array()) - 1
	x := typeNames.Get(strconv.Itoa(typeIndex) + ".String.str").String()
	if x == "" {
		panic("type name not found")
	}
	return x
}
