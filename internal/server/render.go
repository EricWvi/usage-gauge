// Package server renders pages and serves the usage API.
package server

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"usage-gauge/internal/types"
	"usage-gauge/internal/ui"
)

// PageData is the model for both the full page and the cards fragment.
type PageData struct {
	LastSuccessAt   int64
	LastUpdatedText string
	Records         []types.UsageRecord
}

// Renderer wraps the parsed templates.
type Renderer struct {
	tmpl *template.Template
}

// NewRenderer parses the embedded templates with the helper FuncMap.
func NewRenderer() (*Renderer, error) {
	tmpl, err := template.New("").Funcs(funcs).ParseFS(ui.Files, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Renderer{tmpl: tmpl}, nil
}

// RenderPage renders the full HTML document.
func (r *Renderer) RenderPage(d PageData) (string, error) {
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "layout", d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderCards renders only the cards fragment (for /api/usage polling).
func (r *Renderer) RenderCards(d PageData) (string, error) {
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "cards", d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var funcs = template.FuncMap{
	"fmtTime":    fmtTime,
	"utilPct":    utilPct,
	"utilColor":  utilColor,
	"tierMeta":   tierMeta,
}

// fmtTime formats an int64 epoch-ms or an RFC3339 string to a local string.
func fmtTime(v any) string {
	var t time.Time
	switch x := v.(type) {
	case int64:
		if x == 0 {
			return ""
		}
		t = time.UnixMilli(x)
	case int:
		if x == 0 {
			return ""
		}
		t = time.UnixMilli(int64(x))
	case string:
		if x == "" {
			return ""
		}
		parsed, err := time.Parse(time.RFC3339, x)
		if err != nil {
			return x
		}
		t = parsed
	default:
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// utilPct returns a 0-100 percentage, preferring utilization, else used/limit.
func utilPct(util, limit, used float64) float64 {
	var p float64
	switch {
	case util > 0:
		p = util
	case limit > 0:
		p = used / limit * 100
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p
}

// utilColor maps a percentage to a CSS color class.
func utilColor(util, limit, used float64) string {
	switch p := utilPct(util, limit, used); {
	case p >= 90:
		return "danger"
	case p >= 70:
		return "warn"
	default:
		return "ok"
	}
}

// tierMeta returns a human-readable line for a tier.
func tierMeta(t types.UsageTier) string {
	if t.Utilization > 0 {
		return fmt.Sprintf("%.1f%% used", t.Utilization)
	}
	if t.Limit > 0 {
		if t.Unit != "" {
			return fmt.Sprintf("%.0f / %.0f %s", t.Used, t.Limit, t.Unit)
		}
		return fmt.Sprintf("%.0f / %.0f", t.Used, t.Limit)
	}
	return ""
}

// lastUpdatedText formats the global last-success timestamp.
func lastUpdatedText(ms int64) string {
	if ms == 0 {
		return "never"
	}
	return time.UnixMilli(ms).Local().Format("2006-01-02 15:04:05")
}
