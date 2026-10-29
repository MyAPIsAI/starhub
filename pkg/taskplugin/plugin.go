// Package taskplugin runs trusted, synchronous video protocol adapters.
package taskplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/dop251/goja"
)

const (
	MaxSource   = 256 << 10
	MaxJSON     = 1 << 20
	CallTimeout = time.Second
)

var slots = make(chan struct{}, 8)
var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,29}$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type Meta struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	APIVersion int      `json:"apiVersion"`
	Models     []string `json:"models"`
	Auth       string   `json:"auth"`
}

type Plugin struct {
	Meta    Meta
	program *goja.Program
}

type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

type Usage struct {
	Seconds float64 `json:"seconds"`
	N       float64 `json:"n"`
}

func (u Usage) Validate() error {
	if !(u.Seconds > 0 && u.Seconds <= 60 && u.N > 0 && u.N <= 10 && math.Trunc(u.N) == u.N) {
		return errors.New("plugin usage requires seconds in (0,60] and n in (0,10]")
	}
	return nil
}

type Prepared struct {
	Request Request `json:"request"`
	Usage   Usage   `json:"usage"`
}

type Submitted struct {
	TaskID string `json:"taskId"`
}

type Result struct {
	Status     string   `json:"status"`
	Progress   int      `json:"progress"`
	FailReason string   `json:"fail_reason,omitempty"`
	Outputs    []string `json:"outputs,omitempty"`
	Usage      *Usage   `json:"usage,omitempty"`
}

func Compile(ctx context.Context, source string) (*Plugin, error) {
	if len(source) == 0 || len(source) > MaxSource {
		return nil, errors.New("plugin source must be between 1 and 262144 bytes")
	}
	program, err := goja.Compile("plugin.js", "(function(){\n\"use strict\";\n"+source+"\n;return {meta, prepareSubmit, parseSubmit, buildQuery, parseQuery};\n})()", true)
	if err != nil {
		return nil, errors.New("invalid plugin syntax")
	}
	p := &Plugin{program: program}
	if err := p.Call(ctx, "meta", nil, &p.Meta); err != nil {
		return nil, err
	}
	m := p.Meta
	if !keyPattern.MatchString(m.Key) || !versionPattern.MatchString(m.Version) || m.APIVersion != 1 || len(m.Name) == 0 || len(m.Name) > 128 || len(m.Models) == 0 || len(m.Models) > 100 {
		return nil, errors.New("invalid plugin metadata; apiVersion must be 1 and version must be x.y.z")
	}
	if m.Auth != "bearer" && m.Auth != "x-api-key" {
		return nil, errors.New("plugin auth must be bearer or x-api-key")
	}
	seen := map[string]bool{}
	for _, name := range m.Models {
		if strings.TrimSpace(name) != name || name == "" || len(name) > 128 || seen[name] {
			return nil, errors.New("invalid or duplicate plugin model")
		}
		seen[name] = true
	}
	return p, nil
}

// Call uses JSON copies only. No Go services, credentials or network APIs enter JS.
func (p *Plugin) Call(ctx context.Context, hook string, args []any, output any) (err error) {
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return errors.New("plugin execution timed out")
	}
	vm := goja.New()
	vm.SetMaxCallStackSize(128)
	stop := context.AfterFunc(ctx, func() { vm.Interrupt("plugin execution timed out") })
	defer stop()
	defer func() {
		if recover() != nil {
			err = errors.New("plugin execution failed")
		}
	}()
	jsonObject := vm.Get("JSON").ToObject(vm)
	parse, _ := goja.AssertFunction(jsonObject.Get("parse"))
	stringify, _ := goja.AssertFunction(jsonObject.Get("stringify"))
	value, err := vm.RunProgram(p.program)
	if err != nil {
		return errors.New("plugin initialization failed or timed out")
	}
	object := value.ToObject(vm)
	for _, name := range []string{"prepareSubmit", "parseSubmit", "buildQuery", "parseQuery"} {
		if _, ok := goja.AssertFunction(object.Get(name)); !ok {
			return fmt.Errorf("plugin requires function %s", name)
		}
	}
	value = object.Get("meta")
	if hook != "meta" {
		fn, ok := goja.AssertFunction(object.Get(hook))
		if !ok || !slices.Contains([]string{"prepareSubmit", "parseSubmit", "buildQuery", "parseQuery"}, hook) {
			return errors.New("unsupported plugin hook")
		}
		values := make([]goja.Value, len(args))
		for i, arg := range args {
			data, e := json.Marshal(arg)
			if e != nil || len(data) > MaxJSON {
				return errors.New("invalid or oversized plugin input")
			}
			values[i], err = parse(goja.Undefined(), vm.ToValue(string(data)))
			if err != nil {
				return errors.New("invalid plugin input")
			}
		}
		value, err = fn(goja.Undefined(), values...)
		if err != nil {
			return fmt.Errorf("plugin %s failed or timed out", hook)
		}
	}
	encoded, err := stringify(goja.Undefined(), value)
	if err != nil || encoded == nil || goja.IsUndefined(encoded) {
		return errors.New("plugin must return a JSON value")
	}
	data := []byte(encoded.String())
	if len(data) > MaxJSON {
		return errors.New("plugin output too large")
	}
	if err := json.Unmarshal(data, output); err != nil {
		return errors.New("invalid plugin result structure")
	}
	return nil
}

// URL keeps every descriptor beneath the configured base and on the same origin.
func (r Request) URL(base string, method string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid plugin channel base URL")
	}
	path, err := url.Parse(r.Path)
	if err != nil || path == nil || path.IsAbs() || path.Host != "" || path.Fragment != "" || !strings.HasPrefix(r.Path, "/") || strings.HasPrefix(r.Path, "//") || strings.Contains(r.Path, "\\") || r.Method != method {
		return "", errors.New("invalid plugin request path or method")
	}
	for _, segment := range strings.Split(path.Path, "/") {
		if segment == ".." || segment == "." {
			return "", errors.New("plugin path traversal is not allowed")
		}
	}
	for key, value := range r.Headers {
		if !strings.EqualFold(key, "Content-Type") && !strings.EqualFold(key, "Accept") {
			return "", errors.New("plugin headers may only set Content-Type and Accept")
		}
		if strings.ContainsAny(value, "\r\n") || len(value) > 256 {
			return "", errors.New("invalid plugin header value")
		}
	}
	if method == http.MethodPost && (len(r.Body) == 0 || !json.Valid(r.Body)) {
		return "", errors.New("plugin submit requires a JSON body")
	}
	if method == http.MethodGet && len(r.Body) > 0 && string(r.Body) != "null" {
		return "", errors.New("plugin query must not have a body")
	}
	u.RawPath = strings.TrimRight(u.EscapedPath(), "/") + path.EscapedPath()
	u.Path, err = url.PathUnescape(u.RawPath)
	if err != nil {
		return "", errors.New("invalid escaped plugin path")
	}
	u.RawQuery = path.RawQuery
	return u.String(), nil
}
