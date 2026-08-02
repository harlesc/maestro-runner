package devicelab

import (
	_ "embed"

	"github.com/go-rod/rod"
)

// browserJSHelper is injected into Chrome browser pages on Android.
// Separate copy from both the desktop browser driver's jsHelperCode and the mobile
// webViewJSHelper. Includes dialog overrides (page-level CDP deadlocks on native dialogs)
// and the full element-finding + visibility + polling helpers from the desktop driver.
//
//go:embed browser_jshelper.js
var browserJSHelper string

// webViewJSHelper is the JS helper injected into WebView pages.
// This is intentionally a separate copy from the desktop browser CDP driver's jsHelperCode.
// The two drivers (desktop browser vs mobile WebView) are independent — changes to
// desktop browser JS should never affect mobile WebView behavior, and vice versa.
//
//go:embed webview_jshelper.js
var webViewJSHelper string

// evalHelperSource wraps raw JS helper source in a function expression so it
// can run via page.Evaluate(rod.Eval(...)). rod.Eval compiles the given source
// as `(<js>).apply(this, args)` (rod page_eval.go), which only works when the
// source itself evaluates to a function. The helper files are bare assignment
// statements, so the unwrapped call always threw "<obj>.apply is not a
// function" — the assignment still executed (the helper did land), but every
// connect logged a bogus error and suggested the injection had failed.
func evalHelperSource(src string) *rod.EvalOptions {
	return rod.Eval("() => {\n" + src + "\n}")
}
