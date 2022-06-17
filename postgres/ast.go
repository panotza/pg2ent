package postgres

import (
	"golang.org/x/exp/slices"
)

type Table struct {
	Name    string
	Columns []Column
	Indexes []Index

	imports []string
}

func (t *Table) Finalize() {
	slices.SortStableFunc(t.Indexes, func(_, b Index) bool {
		return b.IsUnique
	})
}

type defaultType struct {
	Type  string
	Value string
}

type foreignKey struct {
	Column string
	Table  string
}

type Column struct {
	Name        string
	Type        string
	IsPrimary   bool
	IsNotNull   bool
	IsUnique    bool
	DefaultType *defaultType

	ForeignKey *foreignKey
}

type SchemaType struct {
	Type         string
	GoType       string
	SchemaType   string
	Import       string
	NullableType string
}

type Index struct {
	Columns  []string
	IsUnique bool
}
