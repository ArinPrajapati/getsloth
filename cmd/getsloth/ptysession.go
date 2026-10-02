package main

import "io"

// ptyConn is the host's handle to the wrapped program's terminal: reads
// return its output, writes are keystrokes, and resize changes its grid.
type ptyConn interface {
	io.ReadWriteCloser
	resize(cols, rows int) error
}

// ptyProcess is a ptyConn together with the process tree running behind
// it. killTree terminates the wrapped command and everything it spawned.
type ptyProcess interface {
	ptyConn
	wait() (exitCode int, err error)
	killTree()
}

type softAction int

const (
	softNone softAction = iota
	softReclaim
	softKillViewers
)
