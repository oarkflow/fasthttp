package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oarkflow/fh"
	"github.com/oarkflow/spl"
)

type splEngine struct {
	engine        *spl.Engine
	dir           string
	ext           string
	ssr           bool
	globals       map[string]any
	assets        map[string]string
}

func newSPLEngine(dir string) *splEngine {
	e := spl.New()
	e.SecureMode = false
	e.AutoEscape = true
	e.BaseDir = dir
	return &splEngine{
		engine:  e,
		dir:     dir,
		ext:     ".html",
		globals: make(map[string]any),
		assets:  make(map[string]string),
	}
}

func (e *splEngine) SetGlobals(g map[string]any) {
	for k, v := range g {
		e.globals[k] = v
	}
}

func (e *splEngine) RuntimeJS() string {
	return e.engine.RuntimeJS()
}

func (e *splEngine) HydrationAsset(name string) (string, bool) {
	js, ok := e.assets[name]
	return js, ok
}

func (e *splEngine) HydrationAssets(prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	e.engine.HydrationAssetURL = func(js string) string {
		name := "spl-hydration." + runtimeAssetVersion(js) + ".js"
		e.assets[name] = js
		if prefix == "" {
			return "/" + name
		}
		return prefix + "/" + name
	}
}

func runtimeAssetVersion(src string) string {
	return fmt.Sprintf("%x", []byte(src)[:8])
}

func (e *splEngine) Render(w io.Writer, name string, data any, layout ...string) error {
	input, ok := data.(map[string]any)
	if !ok {
		if data != nil {
			return fmt.Errorf("spl: data must be map[string]any, got %T", data)
		}
		input = nil
	}
	binding := make(map[string]any, len(input)+len(e.globals))
	for k, v := range input {
		binding[k] = v
	}
	for k, v := range e.globals {
		if _, exists := binding[k]; !exists {
			binding[k] = v
		}
	}

	tmplName := normalizeName(name, e.ext)

	if len(layout) > 0 && layout[0] != "" {
		layoutName := normalizeName(layout[0], e.ext)
		tmplPath := filepath.Join(e.dir, tmplName)
		content, err := os.ReadFile(tmplPath)
		if err != nil {
			return fmt.Errorf("spl: read %s: %w", tmplName, err)
		}
		wrapped := fmt.Sprintf("@extends(%q)\n%s", layoutName, string(content))
		var out string
		if e.ssr {
			out, err = e.engine.RenderSSR(wrapped, binding)
		} else {
			out, err = e.engine.Render(wrapped, binding)
		}
		if err != nil {
			return fmt.Errorf("spl: render %s with layout %s: %w", tmplName, layoutName, err)
		}
		_, err = io.WriteString(w, out)
		return err
	}

	var out string
	var err error
	if e.ssr {
		out, err = e.engine.RenderSSRFile(tmplName, binding)
	} else {
		out, err = e.engine.RenderFile(tmplName, binding)
	}
	if err != nil {
		return fmt.Errorf("spl: render %s: %w", tmplName, err)
	}
	_, err = io.WriteString(w, out)
	return err
}

func normalizeName(name, ext string) string {
	clean := filepath.Clean(name)
	if !strings.HasSuffix(clean, ext) {
		clean += ext
	}
	return clean
}

func main() {
	splEngine := newSPLEngine("views")
	splEngine.ssr = true
	splEngine.SetGlobals(map[string]any{"siteName": "SPL File Upload Demo"})
	splEngine.engine.HydrationRuntimeURL = "/static/spl-runtime.min.js"
	splEngine.HydrationAssets("/static/hydration")

	app := fh.New(fh.Config{
		ReadTimeout:        10 * time.Second,
		WriteTimeout:       10 * time.Second,
		MaxRequestBodySize: 32 * 1024 * 1024,
		TemplateEngine:     splEngine,
	})

	app.Get("/static/spl-runtime.min.js", func(c *fh.Ctx) error {
		c.Set("Content-Type", "application/javascript")
		c.Set("Cache-Control", "public, max-age=31536000, immutable")
		return c.SendString(splEngine.RuntimeJS())
	})
	app.Get("/static/hydration/:asset", func(c *fh.Ctx) error {
		asset, ok := splEngine.HydrationAsset(c.Param("asset"))
		if !ok {
			return c.SendStatus(404)
		}
		c.Set("Content-Type", "application/javascript; charset=utf-8")
		c.Set("Cache-Control", "public, max-age=31536000, immutable")
		return c.SendString(asset)
	})

	app.Static("/uploads", "./uploads", fh.StaticConfig{})

	app.Get("/", func(c *fh.Ctx) error {
		data := map[string]any{"title": "File Upload with Reactivity"}

		if savedName := c.Query("file"); savedName != "" {
			sz, _ := strconv.ParseInt(c.Query("size"), 10, 64)
			data["uploadResult"] = map[string]any{
				"success":      true,
				"originalName": c.Query("name"),
				"savedName":    savedName,
				"size":         sz,
				"mimeType":     c.Query("mime"),
				"url":          "/uploads/" + savedName,
				"uploadedAt":   time.Now().Format(time.RFC3339),
			}
		} else if errMsg := c.Query("error"); errMsg != "" {
			data["uploadResult"] = map[string]any{
				"success": false,
				"error":   errMsg,
			}
		} else {
			data["uploadResult"] = nil
		}

		return c.Render("upload", data)
	})

	app.Post("/upload", func(c *fh.Ctx) error {
		file, err := c.FormFile("file")
		if err != nil {
			return c.Redirect("/?error=" + url.QueryEscape(err.Error()))
		}

		if err := os.MkdirAll("uploads", 0755); err != nil {
			return c.Redirect("/?error=" + url.QueryEscape(err.Error()))
		}

		timestamp := time.Now().UnixMilli()
		savedName := fmt.Sprintf("%d_%s", timestamp, file.FileName)
		dstPath := filepath.Join("uploads", savedName)

		if err := c.SaveFile(file, dstPath); err != nil {
			return c.Redirect("/?error=" + url.QueryEscape(err.Error()))
		}

		contentType := file.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		q := url.Values{}
		q.Set("file", savedName)
		q.Set("name", file.FileName)
		q.Set("size", strconv.FormatInt(file.Size, 10))
		q.Set("mime", contentType)
		return c.Redirect("/?" + q.Encode())
	})

	addr := ":8082"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	log.Printf("Listening on %s", addr)
	log.Fatal(app.Listen(addr))
}
