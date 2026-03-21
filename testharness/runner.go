//go:build js

package testharness

import "syscall/js"

// PostMessage sends a typed message to the Worker's parent via postMessage.
func PostMessage(msgType, text string) {
	obj := js.Global().Get("Object").New()
	obj.Set("type", msgType)
	obj.Set("text", text)
	js.Global().Call("postMessage", obj)
}

// PostResult sends a single test result message.
func PostResult(name string, passed bool, output string) {
	obj := js.Global().Get("Object").New()
	obj.Set("type", "result")
	obj.Set("name", name)
	obj.Set("passed", passed)
	obj.Set("output", output)
	js.Global().Call("postMessage", obj)
}

// PostDone signals that all tests have completed.
// The passed flag reflects whether m.Run() returned 0.
func PostDone(passed bool, output string) {
	obj := js.Global().Get("Object").New()
	obj.Set("type", "done")
	obj.Set("passed", passed)
	obj.Set("output", output)
	js.Global().Call("postMessage", obj)
}
