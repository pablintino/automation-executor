package db

import (
	"fmt"
	"strings"
)

type sqlSelectColumnRenderer interface {
	ToSelectClauseSyntax() string
}

type sqlSelectColumn struct {
	columnName string
	rename     string
	table      string
}

type sqlSelectColumnPgp struct {
	sqlSelectColumn
	key string
}

func (s sqlSelectColumn) ToSelectClauseSyntax() string {
	base := s.columnName
	if s.table != "" {
		base = s.table + "." + base
	}
	if s.rename != "" {
		return base + " as " + s.rename
	}
	return base
}

func (s sqlSelectColumnPgp) ToSelectClauseSyntax() string {
	base := s.columnName
	if s.table != "" {
		base = s.table + "." + s.rename
	}
	renderedFunc := fmt.Sprintf("COALESCE(pgp_sym_decrypt(%s, '%s'), '')", base, s.key)
	if s.rename != "" {
		return renderedFunc + " as " + s.rename
	}
	return renderedFunc
}

func newSqlSelectColumn(column, table string) sqlSelectColumn {
	return newSqlSelectColumnRename(column, "", table)
}

func newSqlSelectColumnRename(column string, rename string, table string) sqlSelectColumn {
	return sqlSelectColumn{columnName: column, rename: rename, table: table}
}

func newSqlSelectPgpColumnRename(column string, rename string, key string, table string) sqlSelectColumnPgp {
	return sqlSelectColumnPgp{sqlSelectColumn: newSqlSelectColumnRename(column, rename, table), key: key}
}

type builder struct {
	selectColumns []sqlSelectColumnRenderer
}

func (b *builder) Column(column string) *builder {
	b.selectColumns = append(b.selectColumns, newSqlSelectColumn(column, ""))
	return b
}
func (b *builder) ColumnRenamed(column string, rename string) *builder {
	b.selectColumns = append(b.selectColumns, newSqlSelectColumnRename(column, rename, ""))
	return b
}
func (b *builder) ColumnPgpEnc(column string, rename string, key string) *builder {
	b.selectColumns = append(b.selectColumns, newSqlSelectPgpColumnRename(column, rename, key, ""))
	return b
}

func (b *builder) Build() string {
	columns := make([]string, len(b.selectColumns))
	for i, col := range b.selectColumns {
		columns[i] = col.ToSelectClauseSyntax()
	}
	return strings.Join(columns, ", ")
}

type tableBuilder struct {
	tableName string
	builder   *builder
}

func (b *tableBuilder) Build() string {
	return b.builder.Build()
}

func (b *tableBuilder) ForTable(table string) *tableBuilder {
	return b.builder.ForTable(table)
}

func (b *tableBuilder) Column(column string) *tableBuilder {
	b.builder.selectColumns = append(b.builder.selectColumns, newSqlSelectColumn(column, b.tableName))
	return b
}
func (b *tableBuilder) ColumnRenamed(column string, rename string) *tableBuilder {
	b.builder.selectColumns = append(b.builder.selectColumns, newSqlSelectColumnRename(column, rename, b.tableName))
	return b
}
func (b *tableBuilder) ColumnPgpEnc(column string, rename string, key string) *tableBuilder {
	b.builder.selectColumns = append(b.builder.selectColumns, newSqlSelectPgpColumnRename(column, rename, key, b.tableName))
	return b
}

func NewSqlSelectColumnsBuilder() *builder {
	return &builder{selectColumns: make([]sqlSelectColumnRenderer, 0)}
}

func (b *builder) ForTable(table string) *tableBuilder {
	return &tableBuilder{tableName: table, builder: b}
}
