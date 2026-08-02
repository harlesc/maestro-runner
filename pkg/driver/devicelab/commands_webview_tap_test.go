package devicelab

import (
	"fmt"
	"strings"
	"testing"

	"github.com/devicelab-dev/maestro-runner/pkg/core"
	"github.com/devicelab-dev/maestro-runner/pkg/flow"
	"github.com/devicelab-dev/maestro-runner/pkg/uiautomator2"
	"github.com/go-rod/rod"
)

// Tests for defect H: tapOn's WebView/DOM fallback. On a WebView-hosted
// screen the native accessibility tree may carry no resource-id for the
// DOM nodes at all (verified on device: a uiautomator dump of the
// WebView login form shows resource-id="" on every input), so the
// native UiSelector strategies can never match `id:` — the downstream
// error. tapOn now keeps native precedence and, only when every native
// strategy misses and a WebView is connected, resolves the selector
// against the DOM and taps through CDP.
//
// Non-inertness: TestTapOnIDFallsBackToWebViewDOM fails when the patch
// is reverted (the tap then fails with the native not-found error).
// TestTapOnIDNativeMatchKeepsPrecedence passes both ways — it guards
// the no-behaviour-change property for native screens.

// webTapMockClient fails every FindAndClick (native miss) but keeps the
// rest of mockDeviceLabClient's behaviour.
type webTapMockClient struct {
	mockDeviceLabClient
	findAndClickErr error
}

func (m *webTapMockClient) FindAndClick(strategy, selector string) (*uiautomator2.Element, error) {
	return nil, m.findAndClickErr
}

type fakeWebElement struct {
	info    *core.ElementInfo
	clicked bool
}

func (e *fakeWebElement) Info() *core.ElementInfo { return e.info }
func (e *fakeWebElement) Text() (string, error)   { return e.info.Text, nil }
func (e *fakeWebElement) Input(string) error      { return nil }
func (e *fakeWebElement) Clear() error            { return nil }
func (e *fakeWebElement) Click() error            { e.clicked = true; return nil }

type fakeWebViewManager struct {
	connected bool
	elem      core.Element
	findErr   error
	findCalls int
}

func (m *fakeWebViewManager) connect(cdpInfo *core.CDPInfo, cdpType string) error { return nil }
func (m *fakeWebViewManager) disconnect()                                         { m.connected = false }
func (m *fakeWebViewManager) isConnected() bool                                   { return m.connected }
func (m *fakeWebViewManager) findWebOnce(sel flow.Selector) (core.Element, error) {
	m.findCalls++
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.elem, nil
}
func (m *fakeWebViewManager) findFocusedWeb() (core.Element, error) {
	return nil, fmt.Errorf("no focused element")
}
func (m *fakeWebViewManager) rodPage() *rod.Page { return nil }
func (m *fakeWebViewManager) webViewType() string {
	if !m.connected {
		return ""
	}
	return "webview"
}

func TestTapOnIDFallsBackToWebViewDOM(t *testing.T) {
	client := &webTapMockClient{findAndClickErr: fmt.Errorf("element not found")}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2340}, nil)
	// A connected WebView whose DOM contains the field the native tree
	// does not carry a resource-id for.
	dom := &fakeWebElement{info: &core.ElementInfo{
		Text:   "password",
		Bounds: core.Bounds{X: 55, Y: 539, Width: 874, Height: 102},
	}}
	driver.webView = &fakeWebViewManager{connected: true, elem: dom}
	driver.SetCDPStateFunc(func() *core.CDPInfo {
		return &core.CDPInfo{Available: true, Socket: "webview_devtools_remote_test"}
	})

	result := driver.tapOn(&flow.TapOnStep{
		Selector: flow.Selector{ID: "password"},
		BaseStep: flow.BaseStep{TimeoutMs: 2000},
	})

	if !result.Success {
		t.Fatalf("expected CDP fallback to tap the DOM field, got %v", result.Message)
	}
	if !dom.clicked {
		t.Error("expected the DOM element to be clicked via CDP")
	}
	if driver.webView.(*fakeWebViewManager).findCalls == 0 {
		t.Error("expected the DOM to be probed after the native strategies missed")
	}
}

// Guard: with no WebView connected the failure is the plain native one
// (no behaviour change on native screens).
func TestTapOnIDNativeFailureWithoutWebView(t *testing.T) {
	client := &webTapMockClient{findAndClickErr: fmt.Errorf("element not found")}
	driver := New(client, &core.PlatformInfo{ScreenWidth: 1080, ScreenHeight: 2340}, nil)
	driver.webView = &fakeWebViewManager{connected: false}

	result := driver.tapOn(&flow.TapOnStep{
		Selector: flow.Selector{ID: "password"},
		BaseStep: flow.BaseStep{TimeoutMs: 200},
	})

	if result.Success {
		t.Error("expected failure when native strategies miss and no WebView is connected")
	}
	if !strings.Contains(result.Message, "Element not found") {
		t.Errorf("expected the native not-found error, got %q", result.Message)
	}
}
