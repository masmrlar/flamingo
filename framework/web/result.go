package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"flamingo.me/flamingo/v3/framework/flamingo"
)

type (
	// Result defines the generic web response
	Result interface {
		// Apply executes the response on the http.ResponseWriter
		Apply(ctx context.Context, rw http.ResponseWriter) error
	}

	// Responder generates responses
	Responder struct {
		engine flamingo.TemplateEngine
		router *Router
		logger flamingo.Logger
		debug  bool

		templateForbidden     string
		templateNotFound      string
		templateUnavailable   string
		templateErrorWithCode string
	}

	// Response contains a status and a body
	Response struct {
		Status uint
		Body   io.Reader
		Header http.Header
	}

	// RouteRedirectResponse redirects to a certain route
	RouteRedirectResponse struct {
		Response
		To     string
		Data   map[string]string
		router *Router
	}

	// URLRedirectResponse redirects to a certain URL
	URLRedirectResponse struct {
		Response
		URL *url.URL
	}

	// DataResponse returns a response containing data, e.g. as JSON
	DataResponse struct {
		Response
		Data interface{}
	}

	// RenderResponse renders data
	RenderResponse struct {
		DataResponse
		Template string
		engine   flamingo.TemplateEngine
	}

	// ServerErrorResponse returns a server error, by default http 500
	ServerErrorResponse struct {
		RenderResponse
		Error error
	}
)

// Inject Responder dependencies
func (r *Responder) Inject(router *Router, logger flamingo.Logger, cfg *struct {
	Engine                flamingo.TemplateEngine `inject:",optional"`
	Debug                 bool                    `inject:"config:debug.mode"`
	TemplateForbidden     string                  `inject:"config:flamingo.template.err403"`
	TemplateNotFound      string                  `inject:"config:flamingo.template.err404"`
	TemplateUnavailable   string                  `inject:"config:flamingo.template.err503"`
	TemplateErrorWithCode string                  `inject:"config:flamingo.template.errWithCode"`
}) *Responder {
	r.engine = cfg.Engine
	r.router = router
	r.templateForbidden = cfg.TemplateForbidden
	r.templateNotFound = cfg.TemplateNotFound
	r.templateUnavailable = cfg.TemplateUnavailable
	r.templateErrorWithCode = cfg.TemplateErrorWithCode
	r.logger = logger.WithField("module", "framework.web").WithField("category", "responder")
	r.debug = cfg.Debug

	return r
}

var _ Result = &Response{}

// HTTP Response generator
func (r *Responder) HTTP(status uint, body io.Reader) *Response {
	return &Response{
		Status: status,
		Body:   body,
		Header: make(http.Header),
	}
}

// Apply response
func (r *Response) Apply(c context.Context, w http.ResponseWriter) error {
	for name, vals := range r.Header {
		for _, val := range vals {
			w.Header().Add(name, val)
		}
	}

	w.WriteHeader(int(r.Status))
	if r.Body == nil {
		return nil
	}

	_, err := io.Copy(w, r.Body)
	return err
}

// SetNoCache helper
func (r *Response) SetNoCache() *Response {
	r.Header.Set("Cache-Control", "no-cache, max-age=0, must-revalidate, no-store")
	return r
}

// RouteRedirect generator
func (r *Responder) RouteRedirect(to string, data map[string]string) *RouteRedirectResponse {
	return &RouteRedirectResponse{
		To:     to,
		Data:   data,
		router: r.router,
		Response: Response{
			Status: http.StatusSeeOther,
			Header: make(http.Header),
		},
	}
}

// Apply response
func (r *RouteRedirectResponse) Apply(c context.Context, w http.ResponseWriter) error {
	to, err := r.router.Relative(r.To, r.Data)
	if err != nil {
		return err
	}
	w.Header().Set("Location", to.String())
	return r.Response.Apply(c, w)
}

// Permanent marks a redirect as being permanent (http 301)
func (r *RouteRedirectResponse) Permanent() *RouteRedirectResponse {
	r.Status = http.StatusMovedPermanently
	return r
}

// SetNoCache helper
func (r *RouteRedirectResponse) SetNoCache() *RouteRedirectResponse {
	r.Response.SetNoCache()
	return r
}

// URLRedirect returns a 303 redirect to a given URL
func (r *Responder) URLRedirect(url *url.URL) *URLRedirectResponse {
	return &URLRedirectResponse{
		URL: url,
		Response: Response{
			Status: http.StatusSeeOther,
			Header: make(http.Header),
		},
	}
}

// Apply response
func (r *URLRedirectResponse) Apply(c context.Context, w http.ResponseWriter) error {
	w.Header().Set("Location", r.URL.String())
	return r.Response.Apply(c, w)
}

// Permanent marks a redirect as being permanent (http 301)
func (r *URLRedirectResponse) Permanent() *URLRedirectResponse {
	r.Status = http.StatusMovedPermanently
	return r
}

// SetNoCache helper
func (r *URLRedirectResponse) SetNoCache() *URLRedirectResponse {
	r.Response.SetNoCache()
	return r
}

// Data returns a data response which can be serialized
func (r *Responder) Data(data interface{}) *DataResponse {
	return &DataResponse{
		Data: data,
		Response: Response{
			Status: http.StatusOK,
			Header: make(http.Header),
		},
	}
}

// Apply response
// todo: support more than json
func (r *DataResponse) Apply(c context.Context, w http.ResponseWriter) error {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(r.Data); err != nil {
		return err
	}
	r.Body = buf
	r.Response.Header.Set("Content-Type", "application/json; charset=utf-8")
	return r.Response.Apply(c, w)
}

// Status changes response status code
func (r *DataResponse) Status(status uint) *DataResponse {
	r.Response.Status = status
	return r
}

// SetNoCache helper
func (r *DataResponse) SetNoCache() *DataResponse {
	r.Response.SetNoCache()
	return r
}

// Download returns a download response to handle file downloads
func (r *Responder) Download(data io.Reader, contentType string, fileName string, forceDownload bool) *Response {
	contentDisposition := "inline"
	if forceDownload {
		contentDisposition = "attachement"
	}

	return &Response{
		Status: http.StatusOK,
		Header: http.Header{
			"Content-Type":        []string{contentType},
			"Content-Disposition": []string{contentDisposition + "; filename=" + fileName},
		},
		Body: data,
	}
}

// Render creates a render response, with the supplied template and data
func (r *Responder) Render(tpl string, data interface{}) *RenderResponse {
	return &RenderResponse{
		Template:     tpl,
		engine:       r.engine,
		DataResponse: *r.Data(data),
	}
}

// Apply response
func (r *RenderResponse) Apply(c context.Context, w http.ResponseWriter) error {
	var err error

	if r.engine == nil {
		return r.DataResponse.Apply(c, w)
	}

	if req := RequestFromContext(c); req != nil && r.engine != nil {
		partialRenderer, ok := r.engine.(flamingo.PartialTemplateEngine)
		if partials := req.Request().Header.Get("X-Partial"); partials != "" && ok {
			content, err := partialRenderer.RenderPartials(c, r.Template, r.Data, strings.Split(partials, ","))
			if err != nil {
				return err
			}

			result := make(map[string]string, len(content))
			for k, v := range content {
				buf, err := ioutil.ReadAll(v)
				if err != nil {
					return err
				}
				result[k] = string(buf)
			}

			body, err := json.Marshal(map[string]interface{}{"partials": result, "data": new(GetPartialDataFunc).Func(c).(func() map[string]interface{})()})
			if err != nil {
				return err
			}
			r.Body = bytes.NewBuffer(body)
			r.Header.Set("Content-Type", "application/json; charset=utf-8")
			return r.Response.Apply(c, w)
		}
	}

	r.Header.Set("Content-Type", "text/html; charset=utf-8")
	r.Body, err = r.engine.Render(c, r.Template, r.Data)
	if err != nil {
		return err
	}
	return r.Response.Apply(c, w)
}

// SetNoCache helper
func (r *RenderResponse) SetNoCache() *RenderResponse {
	r.Response.SetNoCache()
	return r
}

// Apply response
func (r *ServerErrorResponse) Apply(c context.Context, w http.ResponseWriter) error {
	return r.RenderResponse.Apply(c, w)
}

// ServerErrorWithCodeAndTemplate error response with template and http status code
func (r *Responder) ServerErrorWithCodeAndTemplate(err error, tpl string, status uint) *ServerErrorResponse {
	errstr := err.Error()
	if r.debug {
		errstr = fmt.Sprintf("%+v", err)
	}
	return &ServerErrorResponse{
		Error: err,
		RenderResponse: RenderResponse{
			Template: tpl,
			engine:   r.engine,
			DataResponse: DataResponse{
				Data: map[string]interface{}{
					"code":  status,
					"error": errstr,
				},
				Response: Response{
					Status: status,
					Header: make(http.Header),
				},
			},
		},
	}
}

// ServerError creates a 500 error response
func (r *Responder) ServerError(err error) *ServerErrorResponse {
	r.getLogger().Error(fmt.Sprintf("%+v\n", err))

	return r.ServerErrorWithCodeAndTemplate(err, r.templateErrorWithCode, http.StatusInternalServerError)
}

// Unavailable creates a 503 error response
func (r *Responder) Unavailable(err error) *ServerErrorResponse {
	r.getLogger().Error(fmt.Sprintf("%+v\n", err))

	return r.ServerErrorWithCodeAndTemplate(err, r.templateUnavailable, http.StatusServiceUnavailable)
}

// NotFound creates a 404 error response
func (r *Responder) NotFound(err error) *ServerErrorResponse {
	r.getLogger().Warn(err)

	return r.ServerErrorWithCodeAndTemplate(err, r.templateNotFound, http.StatusNotFound)
}

// Forbidden creates a 403 error response
func (r *Responder) Forbidden(err error) *ServerErrorResponse {
	r.getLogger().Warn(err)

	return r.ServerErrorWithCodeAndTemplate(err, r.templateForbidden, http.StatusForbidden)
}

// SetNoCache helper
func (r *ServerErrorResponse) SetNoCache() *ServerErrorResponse {
	r.Response.SetNoCache()
	return r
}

// TODO creates a 501 Not Implemented response
func (r *Responder) TODO() *Response {
	return &Response{
		Status: http.StatusNotImplemented,
		Header: make(http.Header),
	}
}

func (r *Responder) getLogger() flamingo.Logger {
	if r.logger != nil {
		return r.logger
	}
	return &flamingo.StdLogger{Logger: *log.New(os.Stdout, "flamingo", log.LstdFlags)}
}
