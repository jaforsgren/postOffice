package textdiff

// Op describes how a line changes when turning the "before" text into the "after" text.
type Op int

const (
	OpEqual Op = iota
	OpDelete
	OpInsert
)

// Line is a single line of a diff together with its change operation.
type Line struct {
	Op   Op
	Text string
}
