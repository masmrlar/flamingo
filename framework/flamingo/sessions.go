package flamingo

import (
	"os"

	"flamingo.me/dingo"
	"flamingo.me/flamingo/v3/framework/config"
	"github.com/boj/redistore"
	"github.com/gomodule/redigo/redis"
	"github.com/gorilla/sessions"
	"github.com/zemirco/memorystore"
)

// SessionModule for session management
type SessionModule struct {
	backend              string
	secret               string
	fileName             string
	secure               bool
	storeLength          int
	maxAge               int
	path                 string
	redisHost            string
	redisPassword        string
	redisIdleConnections int
	redisMaxAge          int
}

// Inject dependencies
func (m *SessionModule) Inject(config *struct {
	// session config is optional to allow usage of the DefaultConfig
	Backend  string `inject:"config:session.backend"`
	Secret   string `inject:"config:session.secret"`
	FileName string `inject:"config:session.file"`
	Secure   bool   `inject:"config:session.cookie.secure"`
	// float64 is used due to the injection as config from json - int is not possible on this
	StoreLength          float64 `inject:"config:session.store.length"`
	MaxAge               float64 `inject:"config:session.max.age"`
	Path                 string  `inject:"config:session.cookie.path"`
	RedisHost            string  `inject:"config:session.redis.host"`
	RedisPassword        string  `inject:"config:session.redis.password"`
	RedisIdleConnections float64 `inject:"config:session.redis.idle.connections"`
	RedisMaxAge          float64 `inject:"config:session.redis.maxAge"`
}) {
	m.backend = config.Backend
	m.secret = config.Secret
	m.fileName = config.FileName
	m.secure = config.Secure
	m.storeLength = int(config.StoreLength)
	m.maxAge = int(config.MaxAge)
	m.path = config.Path
	m.redisHost = config.RedisHost
	m.redisPassword = config.RedisPassword
	m.redisIdleConnections = int(config.RedisIdleConnections)
	m.maxAge = int(config.MaxAge)
}

// Configure DI
func (m *SessionModule) Configure(injector *dingo.Injector) {
	switch m.backend {
	case "redis":
		sessionStore, err := redistore.NewRediStore(int(m.redisIdleConnections), "tcp", m.redisHost, m.redisPassword, []byte(m.secret))
		if err != nil {
			panic(err) // todo: don't panic? fallback?
		}

		sessionStore.SetMaxAge(m.maxAge)
		sessionStore.SetMaxLength(m.storeLength)
		sessionStore.Options.Secure = m.secure
		sessionStore.Options.HttpOnly = true
		sessionStore.Options.Path = m.path
		sessionStore.DefaultMaxAge = m.redisMaxAge

		injector.Bind(new(sessions.Store)).ToInstance(sessionStore)
		injector.Bind(new(redis.Pool)).ToInstance(sessionStore.Pool)
	case "file":
		os.Mkdir(m.fileName, os.ModePerm)
		sessionStore := sessions.NewFilesystemStore(m.fileName, []byte(m.secret))

		sessionStore.MaxLength(m.storeLength)
		sessionStore.MaxAge(m.maxAge)
		sessionStore.Options.Secure = m.secure
		sessionStore.Options.HttpOnly = true
		sessionStore.Options.Path = m.path

		injector.Bind(new(sessions.Store)).ToInstance(sessionStore)
	default: //memory
		sessionStore := memorystore.NewMemoryStore([]byte(m.secret))

		sessionStore.MaxLength(m.storeLength)
		sessionStore.MaxAge(m.maxAge)
		sessionStore.Options.Secure = m.secure
		sessionStore.Options.HttpOnly = true
		sessionStore.Options.Path = m.path

		injector.Bind(new(sessions.Store)).ToInstance(sessionStore)
	}
}

// DefaultConfig for this module
func (m *SessionModule) DefaultConfig() config.Map {
	return config.Map{
		"session.backend":                "memory",
		"session.secret":                 "flamingosecret",
		"session.file":                   "/sessions",
		"session.store.length":           1024 * 1024,
		"session.max.age":                60 * 60 * 24 * 30,
		"session.cookie.secure":          true,
		"session.cookie.path":            "/",
		"session.redis.host":             "redis",
		"session.redis.password":         "",
		"session.redis.idle.connections": 10,
		"session.redis.maxAge":           60 * 60 * 24 * 30,
	}
}
