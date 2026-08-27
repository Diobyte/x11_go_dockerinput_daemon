package main

import (
	"fmt"
	"os"
	"runtime"
)

const destQueueCap = 256

// xOwner serializes dest Xlib onto one OS thread. Legacy v2 still
// uses process-global xmu on its own goroutines and never runs in dest mode.
type xOwner struct {
	jobs chan func()
}

func startXOwner() *xOwner {
	o := &xOwner{jobs: make(chan func(), destQueueCap)}
	go func() {
		runtime.LockOSThread()
		for fn := range o.jobs {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintln(os.Stderr, "xtest-server: dest X owner panic")
						os.Exit(1)
					}
				}()
				fn()
			}()
		}
	}()
	return o
}

func (o *xOwner) call(fn func()) {
	if o == nil {
		panic("xtest-server: dest X owner missing")
	}
	done := make(chan struct{})
	o.jobs <- func() {
		defer close(done)
		fn()
	}
	<-done
}

func (o *xOwner) callErr(fn func() error) error {
	var err error
	o.call(func() { err = fn() })
	return err
}
