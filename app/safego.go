// Package app provides safe goroutine execution with panic recovery.
// This prevents the application from crashing silently when a goroutine panics.
package app

import (
	"runtime/debug"
)

// SafeGo launches a goroutine with panic recovery.
// If the goroutine panics, the panic is logged with a full stack trace
// and the application continues running.
//
// Usage:
//
//	app.SafeGo(func() {
//	    // your code here
//	})
func SafeGo(fn func()) {
	go func() {
		defer RecoverPanic("goroutine")
		fn()
	}()
}

// SafeGoWithName launches a goroutine with panic recovery and a descriptive name.
// The name is included in the log message if a panic occurs.
//
// Usage:
//
//	app.SafeGoWithName("uptimeCounter", func() {
//	    // your code here
//	})
func SafeGoWithName(name string, fn func()) {
	go func() {
		defer RecoverPanic(name)
		fn()
	}()
}

// RecoverPanic recovers from a panic and logs it.
// This should be called with defer at the start of a goroutine.
//
// Usage:
//
//	go func() {
//	    defer app.RecoverPanic("myFunction")
//	    // your code here
//	}()
func RecoverPanic(context string) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		LogError("PANIC RECOVERED [%s]: %v\nStack trace:\n%s", context, r, string(stack))
	}
}

// RecoverPanicWithCallback recovers from a panic, logs it, and calls a callback.
// Useful when you need to perform cleanup or notify another component.
//
// Usage:
//
//	go func() {
//	    defer app.RecoverPanicWithCallback("myFunction", func(err interface{}) {
//	        // handle the panic
//	    })
//	    // your code here
//	}()
func RecoverPanicWithCallback(context string, callback func(interface{})) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		LogError("PANIC RECOVERED [%s]: %v\nStack trace:\n%s", context, r, string(stack))
		if callback != nil {
			callback(r)
		}
	}
}
