package web

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"flamingo.me/flamingo/v3/framework/config"
	"flamingo.me/flamingo/v3/framework/flamingo"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

type (
	// ReverseRouter allows to retrieve urls for controller
	ReverseRouter interface {
		// Relative returns a root-relative URL, starting with `/`
		// if to starts with "/" it will be used as the target, instead of resolving the URL
		Relative(to string, params map[string]string) (*url.URL, error)
		// Absolute returns an absolute URL, with scheme and host.
		// It takes the request to construct as many information as possible
		// if to starts with "/" it will be used as the target, instead of resolving the URL
		Absolute(r *Request, to string, params map[string]string) (*url.URL, error)
	}

	filterProvider func() []Filter
	routesProvider func() []RoutesModule

	Router struct {
		base           *url.URL
		eventRouter    flamingo.EventRouter
		filterProvider filterProvider
		routesProvider routesProvider
		logger         flamingo.Logger
		routerRegistry *RouterRegistry
		configArea     *config.Area
		sessionStore   sessions.Store
		sessionName    string
	}
)

const (
	// FlamingoError is the Controller name for errors
	FlamingoError = "flamingo.error"
	// FlamingoNotfound is the Controller name for 404 notfound
	FlamingoNotfound = "flamingo.notfound"
)

func (r *Router) Inject(
	cfg *struct {
		// base url configuration
		Scheme string `inject:"config:flamingo.router.scheme,optional"`
		Host   string `inject:"config:flamingo.router.host,optional"`
		Path   string `inject:"config:flamingo.router.path,optional"`
	},
	eventRouter flamingo.EventRouter,
	filterProvider filterProvider,
	routesProvider routesProvider,
	logger flamingo.Logger,
	configArea *config.Area,
	sessionStore sessions.Store,
) {
	r.base = &url.URL{
		Scheme: cfg.Scheme,
		Host:   cfg.Host,
		Path:   strings.TrimRight(cfg.Path, "/") + "/",
	}
	r.eventRouter = eventRouter
	r.filterProvider = filterProvider
	r.routesProvider = routesProvider
	r.logger = logger
	r.configArea = configArea
	r.sessionStore = sessionStore
	r.sessionName = "flamingo"
}

func (r *Router) Handler() http.Handler {
	r.routerRegistry = NewRegistry()

	if r.base == nil {
		r.base, _ = url.Parse("/")
	}

	for _, m := range r.routesProvider() {
		m.Routes(r.routerRegistry)
	}

	if r.configArea != nil {
		for _, route := range r.configArea.Routes {
			r.routerRegistry.Route(route.Path, route.Controller)
			if route.Name != "" {
				r.routerRegistry.Alias(route.Name, route.Controller)
			}
		}
	}

	for _, handler := range r.routerRegistry.routes {
		if _, ok := r.routerRegistry.handler[handler.handler]; !ok {
			panic(errors.Errorf("The handler %q has no controller, registered for path %q", handler.handler, handler.path.path))
		}
	}

	return &handler{
		routerRegistry: r.routerRegistry,
		filter:         r.filterProvider(),
		eventRouter:    r.eventRouter,
		logger:         r.logger,
		sessionStore:   r.sessionStore,
		sessionName:    r.sessionName,
		prefix:         strings.TrimRight(r.base.Path, "/"),
	}
}

func (r *Router) ListenAndServe(addr string) error {
	r.eventRouter.Dispatch(context.Background(), &flamingo.ServerStartEvent{})
	defer r.eventRouter.Dispatch(context.Background(), &flamingo.ServerShutdownEvent{})

	return http.ListenAndServe(addr, r.Handler())
}

func (r *Router) Base() *url.URL {
	return r.base
}

// deprecated
func (r *Router) URL(to string, params map[string]string) (*url.URL, error) {
	return r.Relative(to, params)
}

// Relative returns a root-relative URL, starting with `/`
func (r *Router) Relative(to string, params map[string]string) (*url.URL, error) {
	if to == "" {
		a := *r.base
		return &a, nil
	}

	if to[0] == '/' {
		return url.Parse(r.base.Path + strings.TrimLeft(to, "/"))
	}

	p, err := r.routerRegistry.Reverse(to, params)
	if err != nil {
		return nil, err
	}
	return url.Parse(r.base.Path + strings.TrimLeft(p, "/"))
}

// Absolute returns an absolute URL, with scheme and host.
// It takes the request to construct as many information as possible
func (r *Router) Absolute(req *Request, to string, params map[string]string) (*url.URL, error) {
	scheme := r.base.Scheme
	host := r.base.Host

	if scheme == "" {
		if req != nil && req.request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	if host == "" && req != nil {
		host = req.request.Host
	}

	u, err := r.Relative(to, params)
	if err != nil {
		return u, err
	}

	u.Scheme = scheme
	u.Host = host
	return u, nil
}

func dataParams(params map[interface{}]interface{}) RequestParams {
	vars := make(map[string]string, len(params))

	for k, v := range params {
		if k, ok := k.(string); ok {
			switch v := v.(type) {
			case string:
				vars[k] = v
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				vars[k] = strconv.Itoa(int(reflect.ValueOf(v).Int()))
			case float32:
				vars[k] = strconv.FormatFloat(float64(v), 'f', -1, 32)
			case float64:
				vars[k] = strconv.FormatFloat(v, 'f', -1, 64)
			}
		}
	}

	return vars
}

// Data calls a flamingo data controller
func (r *Router) Data(ctx context.Context, handler string, params map[interface{}]interface{}) interface{} {
	ctx, span := trace.StartSpan(ctx, "flamingo/router/data")
	span.Annotate(nil, handler)
	defer span.End()

	req := RequestFromContext(ctx)

	if c, ok := r.routerRegistry.handler[handler]; ok {
		if c.data != nil {
			return c.data(ctx, req, dataParams(params))
		}
		panic(errors.Errorf("%q is not a data Controller", handler))
	}
	panic(errors.Errorf("data Controller %q not found", handler))
}
