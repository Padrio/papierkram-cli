package output

// Column describes one table column: Header is the column title, Path the
// dotted JSON path of the value (as in --fields) and Fmt an optional
// formatter for the value.
type Column struct {
	Header, Path string
	Fmt          func(any) string
}

// Columns are the default columns of a resource for -o table and -o wide.
type Columns struct {
	Table, Wide []Column
}
