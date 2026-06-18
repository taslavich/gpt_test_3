package gin

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

type H map[string]any
type HandlerFunc func(*Context)

type Context struct {
	Writer   http.ResponseWriter
	Request  *http.Request
	params   map[string]string
	handlers []HandlerFunc
	index    int
}

func (c *Context) Header(k, v string)       { c.Writer.Header().Set(k, v) }
func (c *Context) AbortWithStatus(code int) { c.Writer.WriteHeader(code); c.index = len(c.handlers) }
func (c *Context) Next() {
	c.index++
	for c.index < len(c.handlers) {
		c.handlers[c.index](c)
		c.index++
	}
}
func (c *Context) JSON(code int, obj any) {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(code)
	_ = json.NewEncoder(c.Writer).Encode(obj)
}
func (c *Context) PostForm(key string) string { return c.Request.FormValue(key) }
func (c *Context) DefaultPostForm(key, def string) string {
	v := c.PostForm(key)
	if v == "" {
		return def
	}
	return v
}
func (c *Context) FormFile(key string) (*multipart.FileHeader, error) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		return nil, err
	}
	_, h, err := c.Request.FormFile(key)
	return h, err
}
func (c *Context) Param(key string) string { return c.params[key] }
func (c *Context) File(path string)        { http.ServeFile(c.Writer, c.Request, path) }

type Engine struct {
	mux        *http.ServeMux
	middleware []HandlerFunc
}

func Default() *Engine                 { return &Engine{mux: http.NewServeMux()} }
func (e *Engine) Use(m ...HandlerFunc) { e.middleware = append(e.middleware, m...) }
func (e *Engine) Run(addr ...string) error {
	a := ":8080"
	if len(addr) > 0 {
		a = addr[0]
	}
	return http.ListenAndServe(a, e)
}
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) { e.mux.ServeHTTP(w, r) }
func (e *Engine) add(method, pattern string, h HandlerFunc) {
	e.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method && !(method == "GET" && r.Method == "HEAD") {
			http.NotFound(w, r)
			return
		}
		c := &Context{Writer: w, Request: r, params: map[string]string{}, handlers: append(append([]HandlerFunc{}, e.middleware...), h), index: -1}
		c.Next()
	})
}
func (e *Engine) GET(p string, h HandlerFunc) {
	if strings.Contains(p, ":") {
		base := p[:strings.Index(p, ":")-1]
		key := p[strings.Index(p, ":")+1:]
		e.mux.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
			c := &Context{Writer: w, Request: r, params: map[string]string{key: strings.TrimPrefix(r.URL.Path, base+"/")}, handlers: append(append([]HandlerFunc{}, e.middleware...), h), index: -1}
			c.Next()
		})
		return
	}
	e.add("GET", p, h)
}
func (e *Engine) POST(p string, h HandlerFunc) { e.add("POST", p, h) }
func (e *Engine) Static(relativePath, root string) {
	e.mux.Handle(relativePath+"/", http.StripPrefix(relativePath+"/", http.FileServer(http.Dir(filepath.Clean(root)))))
}
