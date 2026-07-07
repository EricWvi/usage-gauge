// Package parser loads and executes user/builtin JS parsers via the goja engine.
//
// Parser resolution order:
//  1. ${CONFIG_DIR}/parser/<name>.js  (user override / new endpoint)
//  2. embedded builtin/<name>.js      (ships with the binary)
//
// Parser contract: the script defines a top-level
//
//	function parse(body, ctx) { return { ... } }
//
// where body is the parsed JSON object and ctx carries httpStatus, rawBody and
// a sanitized endpoint. The returned object must match the UsageResult shape.
package parser

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dop251/goja"

	"usage-gauge/internal/config"
	"usage-gauge/internal/types"
)

//go:embed builtin/*.js
var builtinFS embed.FS

const builtinPrefix = "builtin:"

type compiled struct {
	program *goja.Program
	mtime   time.Time // zero for immutable builtins
}

// Engine loads and runs parser scripts, caching compiled programs.
type Engine struct {
	mu    sync.Mutex
	cache map[string]*compiled
}

// New returns a fresh parser engine.
func New() *Engine {
	return &Engine{cache: make(map[string]*compiled)}
}

// Parse runs the parser named by name against body and ctx.
func (e *Engine) Parse(name string, body map[string]any, ctx types.ParseContext) (types.UsageResult, error) {
	c, err := e.compiled(name)
	if err != nil {
		return types.UsageResult{}, err
	}

	// goja Runtime is not thread-safe; create a fresh one per call.
	vm := goja.New()
	if _, err := vm.RunProgram(c.program); err != nil {
		return types.UsageResult{}, fmt.Errorf("parser %q: run: %w", name, err)
	}

	parseVal := vm.Get("parse")
	if parseVal == nil {
		return types.UsageResult{}, fmt.Errorf("parser %q: missing global function 'parse'", name)
	}
	fn, ok := goja.AssertFunction(parseVal)
	if !ok {
		return types.UsageResult{}, fmt.Errorf("parser %q: 'parse' is not a function", name)
	}

	// Marshal ctx to a map so the JSON field tags (lowerCamelCase) are visible to JS.
	ctxBytes, _ := json.Marshal(ctx)
	var ctxMap map[string]any
	_ = json.Unmarshal(ctxBytes, &ctxMap)

	ret, err := fn(goja.Undefined(), vm.ToValue(body), vm.ToValue(ctxMap))
	if err != nil {
		return types.UsageResult{}, fmt.Errorf("parser %q: %w", name, err)
	}
	if ret == nil || goja.IsNull(ret) || goja.IsUndefined(ret) {
		return types.UsageResult{}, fmt.Errorf("parser %q: returned no value", name)
	}

	// Re-encode via JSON to land in the typed result with the correct field names.
	rawBytes, err := json.Marshal(ret.Export())
	if err != nil {
		return types.UsageResult{}, fmt.Errorf("parser %q: encode result: %w", name, err)
	}
	var r types.UsageResult
	if err := json.Unmarshal(rawBytes, &r); err != nil {
		return types.UsageResult{}, fmt.Errorf("parser %q: decode result: %w", name, err)
	}
	if r.Tiers == nil {
		r.Tiers = []types.UsageTier{}
	}
	if r.QueriedAt == 0 {
		r.QueriedAt = time.Now().UnixMilli()
	}
	return r, nil
}

// compiled resolves a compiled program for name, preferring the user file in
// config/parser/<name>.js and falling back to the embedded builtin.
func (e *Engine) compiled(name string) (*compiled, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if c, err := e.loadUserLocked(name); err == nil {
		return c, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return e.loadBuiltinLocked(name)
}

func (e *Engine) loadUserLocked(name string) (*compiled, error) {
	path := config.ParserPath(name)
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	// Cache hit unless the file changed on disk (handy for dev edits).
	if c := e.cache[name]; c != nil && !fi.ModTime().After(c.mtime) {
		return c, nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	prog, err := goja.Compile(path, string(src), false)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", path, err)
	}
	c := &compiled{program: prog, mtime: fi.ModTime()}
	e.cache[name] = c
	return c, nil
}

func (e *Engine) loadBuiltinLocked(name string) (*compiled, error) {
	key := builtinPrefix + name
	if c := e.cache[key]; c != nil {
		return c, nil
	}
	data, err := builtinFS.ReadFile("builtin/" + name + ".js")
	if err != nil {
		return nil, fmt.Errorf("parser %q: not found in config/parser or builtin", name)
	}
	prog, err := goja.Compile("builtin/"+name+".js", string(data), false)
	if err != nil {
		return nil, fmt.Errorf("parser %q: compile builtin: %w", name, err)
	}
	c := &compiled{program: prog}
	e.cache[key] = c
	return c, nil
}
